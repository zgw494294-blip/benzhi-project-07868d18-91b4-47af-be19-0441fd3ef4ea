package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type Repository struct{ db *sql.DB }

func Open(ctx context.Context, dsn string) (*Repository, error) {
	if strings.TrimSpace(dsn) == "" {
		dsn = "file:cleanroom-monitor.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	repo := &Repository{db: db}
	if err = repo.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err = repo.verifySchema(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Close() error { return r.db.Close() }
