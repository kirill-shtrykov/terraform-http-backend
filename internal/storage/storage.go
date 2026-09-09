package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	log "log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	stateFileExt    = ".tfstate" // Terraform state file extension.
	lockFileExt     = ".lock"    // Lock file extension.
	defaultFileMode = 0o644      // Default permission for files
	defaultDirMode  = 0o755      // Default permission for directory
	testFileName    = "test_rw"  // File name for read/write permission check.
)

var (
	ErrNotDirectory     = errors.New("is not directory")
	ErrEmptyStateName   = errors.New("empty state name")
	ErrInvalidStateName = errors.New("invalid state name")
	ErrPathTraversal    = errors.New("path traversal attempt")
)

// Storage represents Terraform state files storage.
type Storage struct {
	path string
}

// isLocked returns true if lock file exists for given name.
func (s *Storage) isLocked(path string) bool {
	lock := strings.TrimSuffix(path, stateFileExt) + lockFileExt

	info, err := os.Stat(lock) // #nosec G703 -- path validated
	if err != nil || info.IsDir() {
		return false
	}

	return true
}

func (s *Storage) exists(path string) bool {
	info, err := os.Stat(path) // #nosec G703 -- path validated
	if err != nil || info.IsDir() {
		return false
	}

	return true
}

func (s *Storage) lock(path string) error {
	lock := strings.TrimSuffix(path, stateFileExt) + lockFileExt

	if _, err := os.Create(lock); err != nil {
		return fmt.Errorf("failed to create lock file %s: %w", lock, err)
	}

	return nil
}

func (s *Storage) unlock(path string) error {
	lock := strings.TrimSuffix(path, stateFileExt) + lockFileExt

	if err := os.Remove(lock); err != nil {
		return fmt.Errorf("failed to remove lock file %s: %w", path, err)
	}

	return nil
}

// AllStates is an HTTP handler that lists all Terraform state files available in the storage.
func (s *Storage) AllStates(w http.ResponseWriter, _ *http.Request) {
	dir, err := os.Open(s.path)
	if err != nil {
		log.Error("failed to open directory:", "path", s.path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)

		return
	}

	entries, err := dir.ReadDir(0)
	if err != nil {
		log.Error("failed to read directory:", "path", s.path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)

		return
	}

	var states []*State

	for _, e := range entries {
		if filepath.Ext(e.Name()) == stateFileExt {
			name := strings.TrimSuffix(e.Name(), stateFileExt)
			states = append(states, &State{Name: name, Locked: s.isLocked(filepath.Join(s.path, e.Name()))})
		}
	}

	type Result struct {
		Status string   `json:"status"`
		States []*State `json:"states"`
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(Result{Status: "ok", States: states}); err != nil {
		log.Error("failed to encode JSON:", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// HandleState is a root handler for states.
func (s *Storage) HandleState(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := validateStateName(name); err != nil {
		http.Error(w, fmt.Sprintf("Bad Request: %v", err), http.StatusBadRequest)

		return
	}

	log.Debug("Request", "method", r.Method, "name", name)

	handler := map[string]func(http.ResponseWriter, *http.Request, string){
		http.MethodGet:    s.handleGet,
		http.MethodPost:   s.handlePost,
		http.MethodDelete: s.handleDelete,
		"LOCK":            s.handleLock,
		"UNLOCK":          s.handleUnlock,
	}[r.Method]

	if handler == nil {
		log.Warn("unknown method", "method", r.Method, "name", name)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)

		return
	}

	path := filepath.Join(s.path, name+stateFileExt)

	clean := filepath.Clean(path)

	handler(w, r, clean)
}

// handleGet is HTTP handler for GET method.
func (s *Storage) handleGet(w http.ResponseWriter, _ *http.Request, path string) {
	data, err := os.ReadFile(path) // #nosec G703 -- path validated
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "Not Found", http.StatusNotFound)

			return
		}

		log.Error("failed to read file", "path", path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(data); err != nil { // #nosec G705 -- Content-Type is application/json
		log.Error("failed to write response", "path", path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handlePost if HTTP handler for POST method.
func (s *Storage) handlePost(w http.ResponseWriter, r *http.Request, path string) {
	defer r.Body.Close()

	data, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error("failed to read request body", "path", path, "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)

		return
	}

	if !s.exists(path) {
		w.WriteHeader(http.StatusCreated)
	}

	if err := os.WriteFile(path, data, defaultFileMode); err != nil {
		log.Error("failed to write file", "path", path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleDelete is HTTP handler for DELETE method.
func (s *Storage) handleDelete(w http.ResponseWriter, _ *http.Request, path string) {
	if err := os.Remove(path); err != nil {
		log.Error("failed to delete file", "path", path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleLock is HTTP handler for LOCK method.
func (s *Storage) handleLock(w http.ResponseWriter, _ *http.Request, path string) {
	if s.isLocked(path) {
		log.Warn("state already locked", "path", path)
		http.Error(w, "Locked", http.StatusLocked)

		return
	}

	if err := s.lock(path); err != nil {
		log.Error("failed to lock state", "path", path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleUnlock is HTTP handler for UNLOCK method.
func (s *Storage) handleUnlock(w http.ResponseWriter, _ *http.Request, path string) {
	if !s.isLocked(path) {
		log.Warn("state not locked", "path", path)
		http.Error(w, "Conflict", http.StatusConflict)

		return
	}

	if err := s.unlock(path); err != nil {
		log.Error("failed to unlock state", "path", path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func ensureDirectoryExists(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info, nil
	}

	if os.IsNotExist(err) {
		log.Warn("storage directory does not exist:", "path", path)
		log.Debug("creating storage directory " + path)

		if err := os.MkdirAll(path, defaultDirMode); err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", path, err)
		}

		info, err = os.Stat(path)
		if err == nil {
			return info, nil
		}
	}

	return nil, fmt.Errorf("failed to retrieve information for %s: %w", path, err)
}

func validateStateName(name string) error {
	if name == "" {
		return ErrEmptyStateName
	}

	if filepath.Base(name) != name {
		return ErrInvalidStateName
	}

	return nil
}

// New check storage path and retrieves new Storage instance.
func New(path string) (*Storage, error) {
	clean := filepath.Clean(path)

	log.Debug("storage path: " + clean)

	info, err := ensureDirectoryExists(clean)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage %s: %w", clean, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrNotDirectory, clean)
	}

	file := filepath.Join(clean, testFileName)

	fh, err := os.Create(file)
	if err != nil {
		return nil, fmt.Errorf("insufficient permissions for reading and writing in %s: %w", clean, err)
	}

	if err := fh.Close(); err != nil {
		return nil, fmt.Errorf("failed close testfile %s: %w", file, err)
	}

	if err := os.Remove(file); err != nil {
		return nil, fmt.Errorf("failed remove testfile %s: %w", file, err)
	}

	s := &Storage{path: clean}

	return s, nil
}
