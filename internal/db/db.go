package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"aaa2ppp/teams-tasks/internal/lib/logging"
)

type Config struct {
	Addr     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	host, port, _ := strings.Cut(cfg.Addr, ":")
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "3306"
	}

	// DSN: user:password@tcp(host:port)/dbname?parseTime=true
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", cfg.User, cfg.Password, host, port, cfg.DBName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	return db, nil
}

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Transactor sql.DB

func NewTransactor(db *sql.DB) *Transactor { return (*Transactor)(db) }

func (t *Transactor) InTx(ctx context.Context, fn func(ctx context.Context, tx DBTX) error) error {
	db := (*sql.DB)(t)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr != sql.ErrTxDone {
			logging.GetLogger(ctx).Error("rollback", "error", rollbackErr)
		}
	}()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
