package db

import (
	"context"
	"errors"
	"time"

	"aaa2ppp/teams-tasks/internal/lib/logging"

	"github.com/go-sql-driver/mysql"
	"github.com/sony/gobreaker/v2"
)

type CircuitBreakerConfig struct {
	WindowInterval time.Duration
	BucketPeriod   time.Duration
	OpenTimeout    time.Duration
	HalfOpenMaxReq int
	WindowMinReq   int
	FailureRatio   float64
}

// CircuitBreaker для MySQL.
//
// **ВАЖНО:**
//
// Сбоем считается любая ошибка драйвера (*mysql.MySQLError).
// Ожидаемые ошибки драйвера (например, duplicate key) ДОЛЖНЫ быть переопределены
// БЕЗ использования обертывания %w. Обертывание сохраняет тип.
// Breaker будет считать такую ошибку сбоем.
type CircuitBreaker struct {
	cb *gobreaker.CircuitBreaker[struct{}] // stub type not be used
}

func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	cb := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:         "mysql",
		MaxRequests:  uint32(cfg.HalfOpenMaxReq),
		Interval:     cfg.WindowInterval,
		BucketPeriod: cfg.BucketPeriod,
		Timeout:      cfg.OpenTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			requests := counts.Requests - counts.TotalExclusions
			if requests < uint32(cfg.WindowMinReq) {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(requests)
			return failureRatio >= cfg.FailureRatio
		},
		IsSuccessful: func(err error) bool {
			if _, ok := errors.AsType[*mysql.MySQLError](err); ok {
				ctx := context.Background()
				if se, ok := err.(*shuttleError); ok {
					ctx = se.ctx
					err = se.err
				}
				logger := logging.GetLogger(ctx)
				logger.Debug("CircuitBreaker: detected fail", "error", err)
				return false
			}
			return true
		},
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		},
	})
	return &CircuitBreaker{cb}
}

// Обертка, чтобы дотащить контекст запроса до gobreaker.CircuitBreaker
type shuttleError struct {
	ctx context.Context
	err error
}

func (se *shuttleError) Error() string { return se.err.Error() }
func (se *shuttleError) Unwrap() error { return se.err }

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	_, err := cb.cb.Execute(func() (struct{}, error) {
		if err := fn(ctx); err != nil {
			return struct{}{}, &shuttleError{ctx, err}
		}
		return struct{}{}, nil
	})
	if se, ok := err.(*shuttleError); ok {
		return se.err
	}
	return err
}
