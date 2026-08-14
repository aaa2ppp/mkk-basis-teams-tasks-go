package config

import (
	"log/slog"
	"os"
	"time"

	"aaa2ppp/teams-tasks/internal/lib/getval"
	"aaa2ppp/teams-tasks/internal/lib/logging"
	"aaa2ppp/teams-tasks/internal/storage"
)

type Logger = logging.Config
type DB = storage.DBConfig

type Server struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	RequestTimeout  time.Duration
}

type Config struct {
	Logger Logger
	DB     DB
	Server Server
}

const (
	required = true
	optional = false
)

func Load() (Config, error) {
	gv := getval.New(os.LookupEnv)

	cfg := Config{
		Logger: Logger{
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
		},
	}

	return cfg, gv.Err()
}
