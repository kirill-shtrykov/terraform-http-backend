package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const name = "test"

func setupTestStorage(t *testing.T) *Storage {
	t.Helper()

	dir, err := os.MkdirTemp("", "terraform-backend-test")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("failed to initialize storage: %v", err)
	}

	return storage
}

func TestStorageHandleGet(t *testing.T) {
	t.Parallel()

	storage := setupTestStorage(t)
	content := []byte("test content")
	filePath := filepath.Join(storage.path, name+stateFileExt)

	if err := os.WriteFile(filePath, content, defaultFileMode); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	storage.handleGet(w, req, name)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d", res.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if !bytes.Equal(body, content) {
		t.Fatalf("unexpected response body: got %s, want %s", body, content)
	}
}

func TestStorageHandlePost(t *testing.T) {
	t.Parallel()

	storage := setupTestStorage(t)
	content := []byte("new content")

	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(content))
	w := httptest.NewRecorder()

	storage.handlePost(w, req, name)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status code: got %d, want %d", res.StatusCode, http.StatusCreated)
	}

	filePath := filepath.Join(storage.path, name+stateFileExt)

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	if !bytes.Equal(fileContent, content) {
		t.Fatalf("unexpected file content: got %s, want %s", fileContent, content)
	}
}

func TestStorageHandleLockUnlock(t *testing.T) {
	t.Parallel()

	storage := setupTestStorage(t)

	// Lock
	reqLock := httptest.NewRequest("LOCK", "/test", nil)
	wLock := httptest.NewRecorder()

	storage.handleLock(wLock, reqLock, name)

	resLock := wLock.Result()
	defer resLock.Body.Close()

	if resLock.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code for LOCK: got %d, want %d", resLock.StatusCode, http.StatusOK)
	}

	// Unlock
	reqUnlock := httptest.NewRequest("UNLOCK", "/test", nil)
	wUnlock := httptest.NewRecorder()

	storage.handleUnlock(wUnlock, reqUnlock, name)

	resUnlock := wUnlock.Result()
	defer resUnlock.Body.Close()

	if resUnlock.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code for UNLOCK: got %d, want %d", resUnlock.StatusCode, http.StatusOK)
	}
}
