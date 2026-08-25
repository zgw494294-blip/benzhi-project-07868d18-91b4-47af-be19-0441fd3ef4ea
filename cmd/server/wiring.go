package main

import (
	"context"
	"fmt"
	"net/http"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/storage/sqlite"
	"cleanroom-monitor-release/internal/transport/httpapi"
)

type components struct {
	repository *sqlite.Repository
	httpServer *http.Server
}

func wire(ctx context.Context, cfg config) (components, error) {
	dsn := cfg.database
	if cfg.selfcheck {
		dsn = "file:selfcheck?mode=memory&cache=shared&_pragma=foreign_keys(1)"
	}
	repository, err := sqlite.Open(ctx, dsn)
	if err != nil {
		return components{}, fmt.Errorf("初始化数据库: %w", err)
	}
	service := application.NewService(repository, certificate.NewGenerator(), nil)
	api := httpapi.New(service)
	return components{repository: repository, httpServer: &http.Server{Addr: cfg.addr, Handler: api.Handler(), ReadHeaderTimeout: 5_000_000_000}}, nil
}
