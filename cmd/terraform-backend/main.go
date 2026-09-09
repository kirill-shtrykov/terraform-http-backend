package main

import (
	log "log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/kirill-shtrykov/terraform-http-backend/internal/config"
	"github.com/kirill-shtrykov/terraform-http-backend/internal/server"
	"github.com/kirill-shtrykov/terraform-http-backend/internal/storage"
)

var version = "dev"

func setupLogging(debug bool) {
	if debug {
		log.SetLogLoggerLevel(log.LevelDebug)
		log.Debug("debug mode on")
	}
}

func Run() int {
	if len(os.Args) > 1 && slices.Contains([]string{"version", "-version", "--version"}, os.Args[1]) {
		log.Info("Terraform HTTP backend", "version", version)

		return 0
	}

	log.Info("Starting Terraform HTTP backend...")

	cfg, err := config.Load()
	if err != nil {
		log.Error("failed to load config", log.Any("error", err))

		return 1
	}

	setupLogging(cfg.Debug)

	storage, err := storage.New(cfg.Path)
	if err != nil {
		log.Error("failed to init storage", "error", err)

		return 1
	}

	log.Debug("bind address: " + cfg.Address)
	srv := server.New(cfg.Address)

	srv.RegisterHandler("/", http.HandlerFunc(storage.AllStates))
	srv.RegisterHandler("/{name}", http.HandlerFunc(storage.HandleState))

	if err := srv.Run(); err != nil {
		log.Error("error running HTTP server", log.Any("error", err))

		return 1
	}

	log.Info("Finishing Terraform HTTP backend.")

	return 0
}

func main() {
	os.Exit(Run())
}
