package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/viogus/oci-helper-go/internal/config"
	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/handler"
)

func main() {
	// healthcheck mode for docker healthcheck
	if len(os.Args) > 1 && os.Args[1] == "health" {
		resp, err := http.Get("http://localhost:8818/api/config")
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

	// ensure data dir
	if err := os.MkdirAll(cfg.KeysDir, 0700); err != nil {
		log.Fatalf("mkdir keys: %v", err)
	}

	// open db
	store, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
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
		srv.Close()
	}()

	fmt.Printf("[oci-helper] listening on :%s\n", cfg.Port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
