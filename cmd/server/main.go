package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aaa2ppp/teams-tasks/pkg/api/docs"

	"aaa2ppp/teams-tasks/internal/api"
	"aaa2ppp/teams-tasks/internal/config"
	"aaa2ppp/teams-tasks/internal/lib/logging"
	"aaa2ppp/teams-tasks/internal/service"
	"aaa2ppp/teams-tasks/internal/storage/mysqlStorage"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// main godoc
//
//	@title			Сервис для управления задачами внутри команд
//	@version		1.0
//	@license.name	Apache 2.0
//	@basepath		/api
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

func run(ctx context.Context, cfg config.Config) (err error) {
	logger := logging.New(cfg.Logger)

	repo, err := mysqlStorage.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, repo.Close())
	}()

	svc := service.New(repo)
	api := api.New(svc)

	router := http.NewServeMux()
	router.Handle("/swagger/", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))

	if base := docs.SwaggerInfo.BasePath; base == "/" {
		router.Handle("/", api)
	} else {
		router.Handle(base+"/", http.StripPrefix(base, api))
	}

	server := http.Server{
		Handler:      requestTimeout(cfg.Server.RequestTimeout, logging.HTTPLogging(logger, router)),
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

func requestTimeout(d time.Duration, h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		h.ServeHTTP(w, r.WithContext(ctx))
	}
}
