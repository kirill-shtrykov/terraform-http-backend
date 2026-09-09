package storage

// State represents Terraform state file.
type State struct {
	Name   string `json:"name"`
	Locked bool   `json:"locked"`
}
