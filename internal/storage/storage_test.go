package storage_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirill-shtrykov/terraform-http-backend/internal/storage"
)

const statePayloadV4 = `{"version":4}`

func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()

	s, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return s
}

func writeStateFile(t *testing.T, s *storage.Storage, name, content string) {
	t.Helper()

	path := filepath.Join(s.Path(), name+".tfstate")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeStateFile: %v", err)
	}
}

func writeLockFile(t *testing.T, s *storage.Storage, name string) {
	t.Helper()

	path := filepath.Join(s.Path(), name+".lock")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("writeLockFile: %v", err)
	}
}

func newRequestWithContext(method, target string, body io.Reader) *http.Request {
	return httptest.NewRequestWithContext(context.Background(), method, target, body)
}

func makeRequest(t *testing.T, s *storage.Storage, method, name string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	r := newRequestWithContext(method, "/"+name, body)
	r.SetPathValue("name", name)
	s.HandleState(w, r)

	return w
}

// --------------------------------------------------------------------------
// New
// --------------------------------------------------------------------------

func TestNew_ExistingDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	s, err := storage.New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if s.Path() != dir {
		t.Errorf("Path() = %q, want %q", s.Path(), dir)
	}
}

func TestNew_CreatesNewDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "newstorage")

	if _, err := storage.New(dir); err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("New() should have created the directory")
	}
}

func TestNew_CreatesNestedDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "a", "b", "c")

	if _, err := storage.New(dir); err != nil {
		t.Fatalf("New() error creating nested path = %v", err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("New() should have created nested directories")
	}
}

func TestNew_FileInsteadOfDirectory(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "notadir")
	if err != nil {
		t.Fatal(err)
	}

	f.Close()

	if _, err := storage.New(f.Name()); err == nil {
		t.Error("New() expected error when path is a file, got nil")
	}
}

// --------------------------------------------------------------------------
// AllStates
// --------------------------------------------------------------------------

func TestAllStates_Empty(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	w := httptest.NewRecorder()
	s.AllStates(w, newRequestWithContext(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var result struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.Status != "ok" {
		t.Errorf("status field = %q, want ok", result.Status)
	}
}

func TestAllStates_ReturnsStateNames(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	writeStateFile(t, s, "alpha", `{}`)
	writeStateFile(t, s, "beta", `{}`)

	w := httptest.NewRecorder()
	s.AllStates(w, newRequestWithContext(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var result struct {
		States []struct {
			Name string `json:"name"`
		} `json:"states"`
	}

	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(result.States) != 2 {
		t.Errorf("len(states) = %d, want 2", len(result.States))
	}

	names := make(map[string]bool, len(result.States))
	for _, st := range result.States {
		names[st.Name] = true
	}

	for _, want := range []string{"alpha", "beta"} {
		if !names[want] {
			t.Errorf("state %q not found in response", want)
		}
	}
}

func TestAllStates_ContentTypeJSON(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	w := httptest.NewRecorder()
	s.AllStates(w, newRequestWithContext(http.MethodGet, "/", nil))

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestAllStates_ShowsLockedState(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	writeStateFile(t, s, "prod", `{}`)
	writeLockFile(t, s, "prod")

	w := httptest.NewRecorder()
	s.AllStates(w, newRequestWithContext(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var result struct {
		States []struct {
			Name   string `json:"name"`
			Locked bool   `json:"locked"`
		} `json:"states"`
	}

	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, st := range result.States {
		if st.Name == "prod" {
			if !st.Locked {
				t.Error("state 'prod' should be reported as locked")
			}

			return
		}
	}

	t.Error("state 'prod' not found in response")
}

// --------------------------------------------------------------------------
// HandleState — routing
// --------------------------------------------------------------------------

func TestHandleState_EmptyName_BadRequest(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	w := httptest.NewRecorder()
	r := newRequestWithContext(http.MethodGet, "/", nil)
	r.SetPathValue("name", "")
	s.HandleState(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleState_PathTraversal_BadRequest(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	w := makeRequest(t, s, http.MethodGet, "../escape", nil)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleState_UnknownMethod_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	w := makeRequest(t, s, http.MethodPatch, "mystate", nil)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// --------------------------------------------------------------------------
// GET
// --------------------------------------------------------------------------

func TestHandleGet_NotFound(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	w := makeRequest(t, s, http.MethodGet, "nonexistent", nil)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleGet_ReturnsBody(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	writeStateFile(t, s, "mystate", statePayloadV4)

	w := makeRequest(t, s, http.MethodGet, "mystate", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	if got := w.Body.String(); got != statePayloadV4 {
		t.Errorf("body = %q, want %q", got, statePayloadV4)
	}
}

// --------------------------------------------------------------------------
// POST
// --------------------------------------------------------------------------

func TestHandlePost_NewState_Returns201(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	w := makeRequest(t, s, http.MethodPost, "newstate", strings.NewReader(statePayloadV4))

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}

	data, err := os.ReadFile(filepath.Join(s.Path(), "newstate.tfstate"))
	if err != nil {
		t.Fatalf("state file not written: %v", err)
	}

	if string(data) != statePayloadV4 {
		t.Errorf("file content = %q, want %q", string(data), statePayloadV4)
	}
}

func TestHandlePost_ExistingState_Returns200(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	writeStateFile(t, s, "existing", `{"version":3}`)

	newPayload := statePayloadV4

	w := makeRequest(t, s, http.MethodPost, "existing", strings.NewReader(newPayload))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	data, err := os.ReadFile(filepath.Join(s.Path(), "existing.tfstate"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if string(data) != newPayload {
		t.Errorf("file content = %q, want %q", string(data), newPayload)
	}
}

// --------------------------------------------------------------------------
// DELETE
// --------------------------------------------------------------------------

func TestHandleDelete_RemovesFile(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	writeStateFile(t, s, "todelete", `{}`)

	w := makeRequest(t, s, http.MethodDelete, "todelete", nil)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	if _, err := os.Stat(filepath.Join(s.Path(), "todelete.tfstate")); !os.IsNotExist(err) {
		t.Error("state file should have been deleted")
	}
}

// --------------------------------------------------------------------------
// LOCK
// --------------------------------------------------------------------------

func TestHandleLock_CreatesLockFile(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	w := makeRequest(t, s, "LOCK", "mystate", nil)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	if _, err := os.Stat(filepath.Join(s.Path(), "mystate.lock")); os.IsNotExist(err) {
		t.Error("lock file was not created")
	}
}

func TestHandleLock_AlreadyLocked_Returns423(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	writeLockFile(t, s, "mystate")

	w := makeRequest(t, s, "LOCK", "mystate", nil)

	if w.Code != http.StatusLocked {
		t.Errorf("status = %d, want 423", w.Code)
	}
}

// --------------------------------------------------------------------------
// UNLOCK
// --------------------------------------------------------------------------

func TestHandleUnlock_RemovesLockFile(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	writeLockFile(t, s, "mystate")

	w := makeRequest(t, s, "UNLOCK", "mystate", nil)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	if _, err := os.Stat(filepath.Join(s.Path(), "mystate.lock")); !os.IsNotExist(err) {
		t.Error("lock file should have been removed")
	}
}

func TestHandleUnlock_NotLocked_Returns409(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	w := makeRequest(t, s, "UNLOCK", "mystate", nil)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestHandleLockUnlock_Roundtrip(t *testing.T) {
	t.Parallel()

	s := newTestStorage(t)

	writeStateFile(t, s, "prod", `{}`)

	if w := makeRequest(t, s, "LOCK", "prod", nil); w.Code != http.StatusOK {
		t.Fatalf("LOCK status = %d, want 200", w.Code)
	}

	if w := makeRequest(t, s, "LOCK", "prod", nil); w.Code != http.StatusLocked {
		t.Fatalf("second LOCK status = %d, want 423", w.Code)
	}

	if w := makeRequest(t, s, "UNLOCK", "prod", nil); w.Code != http.StatusOK {
		t.Fatalf("UNLOCK status = %d, want 200", w.Code)
	}

	if w := makeRequest(t, s, "UNLOCK", "prod", nil); w.Code != http.StatusConflict {
		t.Fatalf("second UNLOCK status = %d, want 409", w.Code)
	}
}
