package db

import (
	"context"
	"errors"
	"time"

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
	cb *gobreaker.CircuitBreaker[int] // stub type not be used
}

func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	cb := gobreaker.NewCircuitBreaker[int](gobreaker.Settings{
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
			if _, ok := err.(*mysql.MySQLError); ok {
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

func (cb *CircuitBreaker) Execute(fn func() error) error {
	_, err := cb.cb.Execute(func() (int, error) { return 0, fn() })
	return err
}
