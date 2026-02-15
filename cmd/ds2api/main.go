package main

import (
	"net/http"
	"os"
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/server"
)

func main() {
	app := server.NewApp()
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "5001"
	}
	config.Logger.Info("starting ds2api", "port", port)
	if err := http.ListenAndServe("0.0.0.0:"+port, app.Router); err != nil {
		config.Logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
