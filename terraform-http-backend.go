package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	log "log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr  = ":3001"              // Default address to which HTTP server will bind.
	defaultStoragePath = "/var/lib/terraform" // Default path for Terraform state files storage.
	testFileName       = "test_rw"            // File name for read/write permission check.
	stateFileExt       = ".tfstate"           // Terraform state file extension.
	lockFileExt        = ".lock"              // Lock file extension.
	defaultFileMode    = 0o644                // Default permission for files
	defaultDirMode     = 0o755                // Default permission for directory
)

var (
	ErrNotDirectory    = errors.New("is not directory")
	ErrAlreadyLocked   = errors.New("state already locked")
	ErrAlreadyUnlocked = errors.New("state already unlocked")
	ErrAlreadyExists   = errors.New("state already exists")
	ErrNotExists       = errors.New("state does not exists")
)

// stringFromEnv retrieves the value of the environment variable named by the `key`.
// It returns the value if variable present and value not empty.
// Otherwise it returns string value `def`.
func stringFromEnv(key string, def string) string {
	if v := os.Getenv(key); v != "" {
		return strings.TrimSpace(v)
	}

	return def
}

// boolFromEnv retrieves the value of the environment variable named by the `key`.
// It returns the boolean value of the variable if present and valid.
// Otherwise, it returns the default value `def`.
func boolFromEnv(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err == nil {
			return parsed
		}
	}

	return def
}

// Flags represents a command line parameters.
type Flags struct {
	addr  string // The address to which HTTP server will bind.
	path  string // The path to Terraform state files storage.
	debug bool   // Enables debug mode.
}

// parseFlags retrieves the parsed command line parameters.
func parseFlags() *Flags {
	addrHelpText := `
The address to which HTTP server will bind.
Overrides the TF_HTTP_ADDR environment variable if set.
Default = :3001
	`
	pathHelpText := `
The path to Terraform state files storage.
Overrides the TF_HTTP_PATH environment variable if set.
Default = /var/lib/terraform
	`
	debugHelpText := `
Enables debug mode.
Overrides the TF_HTTP_DEBUG environment variable if set.
Default = false
	`

	flags := &Flags{
		addr:  stringFromEnv("TF_HTTP_ADDR", defaultListenAddr),
		path:  stringFromEnv("TF_HTTP_PATH", defaultStoragePath),
		debug: boolFromEnv("TF_HTTP_DEBUG", false),
	}

	flag.StringVar(&flags.addr, "address", flags.addr, strings.TrimSpace(addrHelpText))
	flag.StringVar(&flags.path, "path", flags.path, strings.TrimSpace(pathHelpText))
	flag.BoolVar(&flags.debug, "debug", flags.debug, strings.TrimSpace(debugHelpText))
	flag.Parse()

	return flags
}

// setupLogging enables logging debug mode.
func setupLogging(debug bool) {
	if debug {
		log.SetLogLoggerLevel(log.LevelDebug)
		log.Debug("debug mode on")
	}
}

// State represents Terraform state file.
type State struct {
	Name   string `json:"name"`
	Locked bool   `json:"locked"`
}

// IsLocked returns true if state locked.
func (s *State) IsLocked() bool {
	return s.Locked
}

// Lock state.
// Returns error if state already locked.
func (s *State) Lock() error {
	if s.Locked {
		return ErrAlreadyLocked
	}

	s.Locked = true

	return nil
}

// Unlock state.
// Returns an error if state already unlocked.
func (s *State) Unlock() error {
	if !s.Locked {
		return ErrAlreadyUnlocked
	}

	s.Locked = false

	return nil
}

// States represents list of Terraform state files.
type States []*State

// Returns state and true if States contains state with given name.
func (s *States) State(name string) (*State, bool) {
	for _, state := range *s {
		if state.Name == name {
			return state, true
		}
	}

	return nil, false
}

// Adds a state to the list.
// Returns an error if a state with the same name already exists.
func (s *States) Add(name string) error {
	if _, exists := s.State(name); exists {
		return ErrAlreadyExists
	}

	*s = append(*s, &State{Name: name, Locked: false})

	return nil
}

// Locks state.
// Returns an error if a state with given name already locked or doesn't exists.
func (s *States) Lock(name string) error {
	state, ok := s.State(name)
	if !ok {
		return ErrNotExists
	}

	return state.Lock()
}

func processEntries(entries []os.DirEntry, ext string, action func(name string) error) error {
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ext {
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))

			err := action(name)
			if err != nil {
				return fmt.Errorf("failed to process entry %s: %w", name, err)
			}
		}
	}

	return nil
}

// Storage represents Terraform state files storage.
type Storage struct {
	path string
}

// isLocked returns true if lock file exists for given name.
func (s *Storage) isLocked(name string) bool {
	info, err := os.Stat(filepath.Join(s.path, name+lockFileExt))
	if err != nil || info.IsDir() {
		return false
	}

	return true
}

func (s *Storage) exists(name string) bool {
	info, err := os.Stat(filepath.Join(s.path, name+stateFileExt))
	if err != nil || info.IsDir() {
		return false
	}

	return true
}

// allStates is an HTTP handler that lists all Terraform state files available in the storage.
func (s *Storage) allStates(w http.ResponseWriter, _ *http.Request) {
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

	var states States

	if err := processEntries(entries, stateFileExt, states.Add); err != nil {
		log.Error("failed to create states list:", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)

		return
	}

	if err := processEntries(entries, lockFileExt, states.Lock); err != nil {
		log.Error("failed to update locks for states in list:", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)

		return
	}

	type Result struct {
		Status string  `json:"status"`
		States *States `json:"states"`
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(Result{Status: "ok", States: &states}); err != nil {
		log.Error("failed to encode JSON:", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleState is a root handler for states.
func (s *Storage) handleState(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "Bad Request: missing name", http.StatusBadRequest)

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

	handler(w, r, name)
}

// handleGet is HTTP handler for GET method.
func (s *Storage) handleGet(w http.ResponseWriter, _ *http.Request, name string) {
	filePath := filepath.Join(s.path, name+stateFileExt)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "Not Found", http.StatusNotFound)

			return
		}

		log.Error("failed to read file", "name", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(data); err != nil {
		log.Error("failed to write response", "name", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handlePost if HTTP handler for POST method.
func (s *Storage) handlePost(w http.ResponseWriter, r *http.Request, name string) {
	if s.isLocked(name) {
		log.Warn("file is locked", "name", name)
		http.Error(w, "Locked", http.StatusLocked)

		return
	}

	defer r.Body.Close()

	data, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error("failed to read request body", "name", name, "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)

		return
	}

	filePath := filepath.Join(s.path, name+stateFileExt)

	if !s.exists(name) {
		w.WriteHeader(http.StatusCreated)
	}

	if err := os.WriteFile(filePath, data, defaultFileMode); err != nil {
		log.Error("failed to write file", "name", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleDelete is HTTP handler for DELETE method.
func (s *Storage) handleDelete(w http.ResponseWriter, _ *http.Request, name string) {
	if s.isLocked(name) {
		log.Warn("file is locked", "name", name)
		http.Error(w, "Locked", http.StatusLocked)

		return
	}

	filePath := filepath.Join(s.path, name+stateFileExt)
	if err := os.Remove(filePath); err != nil {
		log.Error("failed to delete file", "name", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleLock is HTTP handler for LOCK method.
func (s *Storage) handleLock(w http.ResponseWriter, _ *http.Request, name string) {
	if s.isLocked(name) {
		log.Warn("state already locked", "name", name)
		http.Error(w, "Locked", http.StatusLocked)

		return
	}

	lockFile := filepath.Join(s.path, name+lockFileExt)
	if _, err := os.Create(lockFile); err != nil {
		log.Error("failed to create lock file", "name", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleUnlock is HTTP handler for UNLOCK method.
func (s *Storage) handleUnlock(w http.ResponseWriter, _ *http.Request, name string) {
	if !s.isLocked(name) {
		log.Warn("state not locked", "name", name)
		http.Error(w, "Conflict", http.StatusConflict)

		return
	}

	lockFile := filepath.Join(s.path, name+lockFileExt)
	if err := os.Remove(lockFile); err != nil {
		log.Error("failed to remove lock file", "name", name, "error", err)
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

		if err := os.Mkdir(path, defaultDirMode); err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", path, err)
		}

		info, err = os.Stat(path)
		if err == nil {
			return info, nil
		}
	}

	return nil, fmt.Errorf("failed to retrieve information for %s: %w", path, err)
}

// NewStorage check storage path and retrieves new Storage instance.
func NewStorage(path string) (*Storage, error) {
	log.Debug("storage path: " + path)

	info, err := ensureDirectoryExists(path)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage %s: %w", path, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrNotDirectory, path)
	}

	file := filepath.Join(path, testFileName)

	fh, err := os.Create(file)
	if err != nil {
		return nil, fmt.Errorf("insufficient permissions for reading and writing in %s: %w", path, err)
	}

	if err := fh.Close(); err != nil {
		return nil, fmt.Errorf("failed close testfile %s: %w", file, err)
	}

	if err := os.Remove(file); err != nil {
		return nil, fmt.Errorf("failed remove testfile %s: %w", file, err)
	}

	s := &Storage{path: path}

	return s, nil
}

func Run() int {
	log.Info("starting Terraform HTTP backend...")

	flags := parseFlags()
	setupLogging(flags.debug)

	storage, err := NewStorage(flags.path)
	if err != nil {
		log.Error("failed to init storage:", "error", err)

		return 1
	}

	http.HandleFunc("/", storage.allStates)
	http.HandleFunc("/{name}", storage.handleState)

	log.Debug("bind address: " + flags.addr)

	srv := http.Server{
		Addr:              flags.addr,
		ReadTimeout:       1 * time.Second,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       1 * time.Minute,
		ReadHeaderTimeout: 1 * time.Second,
		Handler:           nil,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Error("error running HTTP server:", log.Any("error", err))

		return 1
	}

	return 0
}

func main() {
	os.Exit(Run())
}
