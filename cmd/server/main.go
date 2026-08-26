package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"aaa2ppp/teams-tasks/internal/config"
	database "aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/features/sign"
	"aaa2ppp/teams-tasks/internal/features/tasks"
	"aaa2ppp/teams-tasks/internal/features/teams"
	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/lib/logging"
	"aaa2ppp/teams-tasks/pkg/api/docs"

	"github.com/redis/go-redis/v9"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"github.com/ulule/limiter/v3"
	limiterLib "github.com/ulule/limiter/v3/drivers/middleware/stdlib"
	limiterRedis "github.com/ulule/limiter/v3/drivers/store/redis"
)

// main godoc
//
//	@title			Сервис для управления задачами внутри команд
//	@version		1.0
//	@license.name	Apache 2.0
//	@basepath		/api/v1
func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg); err != nil {
		slog.Error("abnormal shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server shutdown successfully")
}

const (
	rdbCacheDB   = 0
	rdbLimiterDB = 1
)

func run(ctx context.Context, cfg config.Config) (err error) {
	logger := logging.New(cfg.Log)
	tokens := auth.New(cfg.Auth)

	db, err := database.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, db.Close())
	}()

	cd := database.NewCircuitBreaker(cfg.DB.CircuitBreaker)

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       rdbCacheDB,
	})
	defer rdb.Close() //nolint:errcheck

	signAPI := newSignAPI(tokens, db, cd)
	teamsAPI := newTeamsAPI(tokens, db, cd)
	tasksAPI := newTasksAPI(tokens, db, cd, rdb)

	router := http.NewServeMux()
	base := strings.TrimRight(docs.SwaggerInfo.BasePath, "/")
	router.Handle(base+"/", http.StripPrefix(base, signAPI))
	router.Handle(base+"/teams/", http.StripPrefix(base, teamsAPI))
	router.Handle(base+"/tasks/", http.StripPrefix(base, tasksAPI))

	swagger := httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json"))
	router.Handle("/swagger/", swagger)

	var handler http.Handler = router

	if cfg.Server.RateLimit > 0 {
		rdbLimiter := redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       rdbLimiterDB,
		})
		defer rdbLimiter.Close() //nolint:errcheck

		rate := limiter.Rate{
			Period: 1 * time.Second,
			Limit:  int64(cfg.Server.RateLimit),
		}
		store, err := limiterRedis.NewStore(rdbLimiter)
		if err != nil {
			return fmt.Errorf("rate limiter store: %w", err)
		}
		limiterInst := limiter.New(store, rate)

		handler = limiterLib.NewMiddleware(limiterInst).Handler(handler)
	}

	handler = logging.Middleware(logger, handler)
	handler = requestTimeout(cfg.Server.RequestTimeout, handler)

	server := http.Server{
		Handler:      handler,
		Addr:         cfg.Server.Addr,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	done := make(chan error, 1)
	go func() {
		defer close(done)
		slog.Info("startup server", "addr", server.Addr)
		done <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown server", "cause", context.Cause(ctx))
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-done:
		return err
	}
}

func newSignAPI(tokens *auth.Tokens, db *database.DB, cb *database.CircuitBreaker) http.Handler {
	return sign.NewAPI(
		sign.NewBreaker(
			cb,
			sign.NewService(
				sign.NewStorage(db),
				db,
				tokens,
			),
		),
	)

}

func newTeamsAPI(tokens *auth.Tokens, db *database.DB, cb *database.CircuitBreaker) http.Handler {
	return tokens.Middleware(
		teams.NewAPI(
			teams.NewBreaker(
				cb,
				teams.NewService(
					teams.NewStorage(db),
					db,
				),
			),
		),
	)
}

func newTasksAPI(tokens *auth.Tokens, db *database.DB, cb *database.CircuitBreaker, rdb *redis.Client) http.Handler {
	return tokens.Middleware(
		tasks.NewAPI(
			tasks.NewBreaker(
				cb,
				tasks.NewService(
					tasks.NewStorage(db),
					db,
					tasks.NewCache(rdb, 5*time.Minute),
				),
			),
		),
	)

}

func requestTimeout(d time.Duration, h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		h.ServeHTTP(w, r.WithContext(ctx))
	}
}
