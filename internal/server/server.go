package server

import (
	"context"
	"net/http"
	"time"

	"a.sidnev/internal/api"
	"a.sidnev/internal/logging"
)

type Server struct {
	httpServer *http.Server
}

func New(addr string) *Server {
	router := api.NewRouter()
	logger := logging.NewLogger()
	handler := logger.Middleware(router)

	return &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
	}
}

func (s *Server) Run() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}
