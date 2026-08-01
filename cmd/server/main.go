// Package main provides the oci-helper HTTP server entry point.
//
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/viogus/oci-helper-go/internal/config"
	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/handler"
)

func main() {
	// healthcheck mode for docker healthcheck
	if len(os.Args) > 1 && os.Args[1] == "health" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8818"
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://localhost:" + port + "/api/health")
		if err != nil {
			os.Exit(1)
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			os.Exit(0)
		}
		os.Exit(1)
	}

	cfg := config.Load()

	// Debug server (pprof) on separate port, only if configured
	if cfg.DebugPort != "" {
		go func() {
			log.Printf("[debug] pprof listening on 127.0.0.1:%s", cfg.DebugPort)
			if err := http.ListenAndServe("127.0.0.1:"+cfg.DebugPort, nil); err != nil {
				log.Printf("[debug] pprof server: %v", err)
			}
		}()
	}

	// set up dual logging: stderr + log file
	var logWriter io.Writer = os.Stderr
	if cfg.LogFile != "" {
		logDir := filepath.Dir(cfg.LogFile)
		if err := os.MkdirAll(logDir, 0755); err == nil {
			f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err == nil {
				logWriter = io.MultiWriter(os.Stderr, f)
				defer f.Close()
			}
		}
	}
	log.SetOutput(logWriter)

	// Wire slog to the same writer with the configured log level.
	var slogLevel slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(logWriter, &slog.HandlerOptions{
		Level: slogLevel,
	})))

	// ensure keys dir exists and is writable (nobody user in container)
	if err := os.MkdirAll(cfg.KeysDir, 0700); err != nil {
		log.Printf("warn: cannot create keys dir %s: %v", cfg.KeysDir, err)
	}
	if err := os.Chmod(cfg.KeysDir, 0700); err != nil {
		log.Printf("warn: cannot set keys dir permission %s: %v (PEM upload may fail)", cfg.KeysDir, err)
	}

	// Fail fast with an actionable message when the data volume is not
	// writable by the container user (the image runs as UID 65534).
	if cfg.DBPath != ":memory:" {
		dbDir := filepath.Dir(cfg.DBPath)
		probe, err := os.CreateTemp(dbDir, ".oci-helper-write-test-*")
		if err != nil {
			log.Fatalf("data directory %s is not writable by UID %d: %v\n"+
				"Fix on host: sudo chown -R 65534:65534 <host-data-dir>\n"+
				"Or in docker-compose add: user: \"0:0\"", dbDir, os.Getuid(), err)
		}
		_ = os.Remove(probe.Name())
	}

	// open db
	store, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db %s: %v (check volume permissions)", cfg.DBPath, err)
	}
	defer store.Close()

	// create server
	server := handler.New(cfg, store)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// graceful shutdown
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down...")

		// Shut down background workers first with a timeout.
		workerCtx, workerCancel := context.WithTimeout(context.Background(), 30*time.Second)
		done := make(chan struct{})
		go func() { server.Shutdown(); close(done) }()
		select {
		case <-done:
		case <-workerCtx.Done():
			log.Println("background worker shutdown timed out")
		}
		workerCancel()

		// Use a fresh context for HTTP server shutdown to avoid racing
		// with the already-expired worker timeout.
		httpCtx, httpCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer httpCancel()
		if err := srv.Shutdown(httpCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	fmt.Printf("[oci-helper] listening on :%s\n", cfg.Port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
