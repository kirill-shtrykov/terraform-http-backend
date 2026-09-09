package storage

// Export_test.go exposes internal symbols for black-box tests in the storage_test package.
// Compiled only during `go test`.

func (s *Storage) Path() string { return s.path }
