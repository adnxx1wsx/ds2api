package handler

import (
	"net/http"
	"sync"

	"ds2api/internal/server"
)

var (
	once sync.Once
	app  *server.App
)

func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(func() {
		app = server.NewApp()
	})
	app.Router.ServeHTTP(w, r)
}
