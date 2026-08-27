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
type DBCircuitBreaker = db.CircuitBreakerConfig
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

type Cache struct {
	TasksTLL time.Duration
}

type Config struct {
	Log    Log
	DB     DB
	Server Server
	Auth   Auth
	Redis  Redis
	Cache  Cache
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
			CircuitBreaker: DBCircuitBreaker{
				WindowInterval: gv.Duration("DB_CB_WINDOW_INTERVAL", optional, 60*time.Second),
				BucketPeriod:   gv.Duration("DB_CB_BUCKET_PERIOD", optional, 10*time.Second),
				OpenTimeout:    gv.Duration("DB_CB_OPEN_TIMEOUT", optional, 30*time.Second),
				HalfOpenMaxReq: gv.Int("DB_CB_HALFOPEN_MAX_REQ", optional, 3),
				WindowMinReq:   gv.Int("DB_CB_WINDOW_MIN_REQ", optional, 10),
				FailureRatio:   gv.Float("DB_CB_FAILURE_RATIO", optional, 0.5),
			},
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
		Cache: Cache{
			TasksTLL: gv.Duration("CACHE_TASKS_TTL", optional, 5*time.Minute),
		},
	}

	return cfg, gv.Err()
}
