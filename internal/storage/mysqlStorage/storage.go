package mysqlStorage

import (
	"aaa2ppp/teams-tasks/internal/storage"
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLRepo sql.DB

func (repo *MySQLRepo) db() *sql.DB { return (*sql.DB)(repo) }

func Open(ctx context.Context, cfg storage.DBConfig) (*MySQLRepo, error) {
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

	return (*MySQLRepo)(db), nil
}

func (repo *MySQLRepo) Close() error {
	return repo.db().Close()
}
