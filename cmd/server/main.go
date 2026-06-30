// Package main provides the oci-helper HTTP server entry point.
//
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
		resp, err := http.Get("http://localhost:" + port + "/api/config")
		if err != nil {
			os.Exit(1)
		}
		resp.Body.Close()
		if resp.StatusCode == 401 || resp.StatusCode == 200 {
			os.Exit(0)
		}
		os.Exit(1)
	}

	cfg := config.Load()

	// set up dual logging: stderr + log file
	if cfg.LogFile != "" {
		logDir := filepath.Dir(cfg.LogFile)
		if err := os.MkdirAll(logDir, 0755); err == nil {
			f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err == nil {
				log.SetOutput(io.MultiWriter(os.Stderr, f))
				defer f.Close()
			}
		}
	}

	// ensure keys dir exists and is writable (nobody user in container)
	if err := os.MkdirAll(cfg.KeysDir, 0777); err != nil {
		log.Printf("warn: cannot create keys dir %s: %v", cfg.KeysDir, err)
	}
	if err := os.Chmod(cfg.KeysDir, 0777); err != nil {
		log.Printf("warn: cannot set keys dir permission %s: %v (PEM upload may fail)", cfg.KeysDir, err)
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
		Addr:         ":" + cfg.Port,
		Handler:      server.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// graceful shutdown
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	fmt.Printf("[oci-helper] listening on :%s\n", cfg.Port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
