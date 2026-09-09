# terraform-http-backend

[![CI](https://github.com/kirill-shtrykov/terraform-http-backend/actions/workflows/ci.yml/badge.svg)](https://github.com/kirill-shtrykov/terraform-http-backend/actions/workflows/ci.yml)

A simple HTTP backend for Terraform using the file system for `tfstate` storage, written in Go.

## Overview

`terraform-http-backend` implements the [Terraform HTTP backend protocol](https://developer.hashicorp.com/terraform/language/settings/backends/http).
It stores state files on the local file system and supports locking, so multiple team members can safely share remote state.

## Installation

### From release packages

Pre-built DEB and RPM packages are available on the [Releases](https://github.com/kirill-shtrykov/terraform-http-backend/releases) page.

```bash
# Debian / Ubuntu
dpkg -i terraform-backend_<version>_amd64.deb

# RHEL / Rocky / Fedora
rpm -i terraform-backend-<version>-1.x86_64.rpm
```

### From source

```bash
go install github.com/kirill-shtrykov/terraform-http-backend/cmd/terraform-backend@latest
```

## Configuration

Configuration is resolved in order of priority (highest to lowest):

| Priority | Source |
|---|---|
| 1 | CLI flags |
| 2 | Environment variables |
| 3 | HCL config file |

### HCL config file

Default location: `/var/lib/terraform-backend/config.hcl`

```hcl
# The address to which HTTP server will bind.
# address = "127.0.0.1:3001"

# The path to Terraform state files storage.
# path = "/var/lib/terraform-backend/state"

# Enables debug mode.
# debug = false
```

### CLI flags

```
-address string   Address to bind the HTTP server (default "127.0.0.1:3001")
-path    string   Path to the state files directory (default "/var/lib/terraform-backend/state")
-config  string   Path to the HCL config file (default "/var/lib/terraform-backend/config.hcl")
-debug            Enable debug logging
-version          Print version and exit
```

### Environment variables

| Variable | Description | Default |
|---|---|---|
| `TF_HTTP_ADDR` | Bind address | `127.0.0.1:3001` |
| `TF_HTTP_PATH` | State files directory | `/var/lib/terraform-backend/state` |
| `TF_HTTP_CONFIG` | Path to config file | `/var/lib/terraform-backend/config.hcl` |
| `TF_HTTP_DEBUG` | Enable debug logging | `false` |

## Running

### Directly

```bash
terraform-backend -address 0.0.0.0:3001 -path /data/tfstate
```

### As a systemd service

The packages install a ready-to-use unit file and automatically create the `terraform` system user and the state directory `/var/lib/terraform-backend/state`. After installation:

```bash
systemctl enable --now terraform-backend
```

## Terraform configuration

```hcl
terraform {
  backend "http" {
    address        = "http://127.0.0.1:3001/my-state"
    lock_address   = "http://127.0.0.1:3001/my-state"
    unlock_address = "http://127.0.0.1:3001/my-state"
    lock_method    = "LOCK"
    unlock_method  = "UNLOCK"
  }
}
```

## API

| Method   | Path      | Description                   |
|----------|-----------|-------------------------------|
| `GET`    | `/{name}` | Retrieve state                |
| `POST`   | `/{name}` | Save state                    |
| `DELETE` | `/{name}` | Delete state                  |
| `LOCK`   | `/{name}` | Lock state (returns 423 if already locked) |
| `UNLOCK` | `/{name}` | Unlock state                  |
| `GET`    | `/`       | List all states with lock status |

## Development

### Prerequisites

- Go 1.25+
- [Task](https://taskfile.dev)
- `golangci-lint` (for linting)
- `dpkg-deb` (for DEB builds)
- `rpmbuild` (for RPM builds)

### Tasks

```bash
task build   # Build binary to dist/terraform-backend
task test    # Run unit tests
task lint    # Run golangci-lint with auto-fix
task deb     # Build DEB package to dist/
task rpm     # Build RPM package to dist/
```

## CI / CD

| Workflow | Trigger | Description |
|---|---|---|
| **CI** | PR → `main` | Runs linter and tests; posts coverage report to the PR |
| **Release** | GitHub Release created | Builds DEB and RPM packages and uploads them as release assets |
