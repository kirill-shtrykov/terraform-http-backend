package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

const (
	defaultAddress  = "127.0.0.1:3001" // Default address to which HTTP server will bind.
	addressHelpText = `
The address to which HTTP server will bind.
Overrides the TF_HTTP_ADDR environment variable if set.
Default = 127.0.0.1:3001
	`
	defaultPath  = "/var/lib/terraform-backend/state" // Default path for Terraform state files storage.
	pathHelpText = `
The path to Terraform state files storage.
Overrides the TF_HTTP_PATH environment variable if set.
Default = /var/lib/terraform-backend/state
	`
	defaultConfigPath = "/var/lib/terraform-backend/config.hcl"
	configHelpText    = `
Path to config file.
Overrides the TF_HTTP_CONFIG environment variable if set.
Default = "/var/lib/terraform-backend/config.hcl"
`
	debugHelpText = `
Enables debug mode.
Overrides the TF_HTTP_DEBUG environment variable if set.
Default = false
	`
)

var ErrInvalidFlags = errors.New("invalid flags")

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

func isFlagPassed(fs *flag.FlagSet, name string) bool {
	found := false

	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})

	return found
}

type Config struct {
	Address string `hcl:"address,optional"`
	Path    string `hcl:"path,optional"`
	Debug   bool   `hcl:"debug,optional"`
}

func fromHCL(path string) (Config, error) {
	var cfg Config

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}

		return Config{}, fmt.Errorf("stat config: %w", err)
	}

	if err := hclsimple.DecodeFile(path, nil, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to read HCL config %s: %w", path, err)
	}

	return cfg, nil
}

func newFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Global flags:\n")
		fs.PrintDefaults()
	}

	return fs
}

func registerFlags(
	fs *flag.FlagSet,
	address, path, configPath *string,
	debug *bool,
) {
	fs.StringVar(address, "address", "", addressHelpText)
	fs.StringVar(path, "path", "", pathHelpText)
	fs.StringVar(configPath, "config", stringFromEnv("TF_HTTP_CONFIG", defaultConfigPath), configHelpText)
	fs.BoolVar(debug, "debug", false, debugHelpText)
}

func applyStringOverrides(cfg *Config, fs *flag.FlagSet, address, path string) {
	if isFlagPassed(fs, "address") {
		cfg.Address = address
	}

	if isFlagPassed(fs, "path") {
		cfg.Path = path
	}
}

func applyBoolOverrides(cfg *Config, fs *flag.FlagSet, debug bool) {
	cfg.Debug = boolFromEnv("TF_HTTP_DEBUG", false)

	if isFlagPassed(fs, "debug") {
		cfg.Debug = debug
	}
}

func Load() (Config, error) {
	var (
		configPath string
		address    string
		path       string
		debug      bool
	)

	fs := newFlagSet()
	registerFlags(fs, &address, &path, &configPath, &debug)

	if err := fs.Parse(os.Args[1:]); err != nil {
		return Config{}, fmt.Errorf("failed to parse flags: %w", err)
	}

	cfg, err := fromHCL(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read HCL: %w", err)
	}

	// Apply env defaults for fields not set by HCL.
	if cfg.Address == "" {
		cfg.Address = stringFromEnv("TF_HTTP_ADDR", defaultAddress)
	}

	if cfg.Path == "" {
		cfg.Path = stringFromEnv("TF_HTTP_PATH", defaultPath)
	}

	// CLI flags override everything.
	applyStringOverrides(&cfg, fs, address, path)
	applyBoolOverrides(&cfg, fs, debug)

	return cfg, nil
}
