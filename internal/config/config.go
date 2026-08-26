package config

import (
	"log/slog"
	"os"
	"time"

	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/lib/getval"
	"aaa2ppp/teams-tasks/internal/lib/logging"
)

type Log = logging.Config
type DB = db.Config
type Auth = auth.Config

type Server struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	RequestTimeout  time.Duration
	RateLimit       int
}

type Redis struct {
	Addr     string
	Password string
	Timeout  time.Duration
}

type Config struct {
	Log    Log
	DB     DB
	Server Server
	Auth   Auth
	Redis  Redis
}

const (
	required = true
	optional = false
)

func Load() (Config, error) {
	gv := getval.New(os.LookupEnv)

	cfg := Config{
		Log: Log{
			Level:     gv.LogLevel("LOG_LEVEL", optional, slog.LevelInfo),
			Plaintext: gv.Bool("LOG_PLAINTEXT", optional, false),
		},
		DB: DB{
			Addr:     gv.String("DB_ADDR", required, ""),
			User:     gv.String("DB_USER", optional, "app-user"),
			Password: gv.String("DB_PASSWORD", required, ""),
			DBName:   gv.String("DB_NAME", optional, "app-db"),
		},
		Server: Server{
			Addr:            gv.String("SERVER_ADDR", required, ""),
			ReadTimeout:     gv.Duration("SERVER_READ_TIMEOUT", optional, 5*time.Second),
			WriteTimeout:    gv.Duration("SERVER_WRITE_TIMEOUT", optional, 5*time.Second),
			RequestTimeout:  gv.Duration("SERVER_REQUEST_TIMEOUT", optional, 5*time.Second),
			ShutdownTimeout: gv.Duration("SERVER_SHUTDOWN_TIMEOUT", optional, 10*time.Second),
			RateLimit:       gv.Int("SERVER_RATE_LIMIT", optional, 30),
		},
		Auth: Auth{
			Secret:        gv.String("AUTH_SECRET", required, ""),
			TokenHeader:   gv.String("AUTH_TOKEN_HEADER", optional, "X-Authtoken"),
			TokenLifetime: gv.Duration("AUTH_TOKEN_LIFETIME", optional, 15*time.Minute),
		},
		Redis: Redis{
			Addr:     gv.String("REDIS_ADDR", required, ""),
			Password: gv.String("REDIS_PASSWORD", required, ""),
			Timeout:  gv.Duration("REDIS_TIMEOUT", optional, 5*time.Second),
		},
	}

	return cfg, gv.Err()
}
