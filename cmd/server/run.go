package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func run(ctx context.Context, cfg config) error {
	parts, err := wire(ctx, cfg)
	if err != nil {
		return err
	}
	defer parts.repository.Close()
	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.addr, err)
	}
	if cfg.selfcheck {
		return runSelfcheck(ctx, listener, parts.httpServer)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- parts.httpServer.Serve(listener) }()
	log.Printf("cleanroom monitor release listening on %s", listener.Addr())
	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := parts.httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
