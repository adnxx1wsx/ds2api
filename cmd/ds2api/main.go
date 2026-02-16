package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/server"
	"ds2api/internal/webui"
)

func main() {
	webui.EnsureBuiltOnStartup()
	_ = auth.AdminKey()
	app := server.NewApp()
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "5001"
	}

	srv := &http.Server{
		Addr:    "0.0.0.0:" + port,
		Handler: app.Router,
	}

	// Start server in a goroutine so we can listen for shutdown signals.
	go func() {
		config.Logger.Info("starting ds2api", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			config.Logger.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal (Ctrl+C / SIGTERM).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	sig := <-quit
	config.Logger.Info("shutdown signal received", "signal", sig.String())

	// Graceful shutdown: allow up to 10 seconds for in-flight requests to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		config.Logger.Error("graceful shutdown failed, forcing exit", "error", err)
		os.Exit(1)
	}
	config.Logger.Info("server gracefully stopped")
}
