package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"a.sidnev/internal/server"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: server <listen-address>")
	}
	listenAddr := os.Args[1]

	srv := server.New(listenAddr)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errChan := make(chan error, 1)
	go func() {
		if err := srv.Run(); err != nil {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutting down server...")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	case err := <-errChan:
		log.Fatalf("Server error: %v", err)
	}
}