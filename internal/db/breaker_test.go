package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"aaa2ppp/teams-tasks/internal/db"

	"github.com/aaa2ppp/be"
	"github.com/go-sql-driver/mysql"
	"github.com/sony/gobreaker/v2"
)

func TestCircuitBreakerLogic(t *testing.T) {
	cfg := db.CircuitBreakerConfig{
		WindowInterval: 100 * time.Millisecond,
		BucketPeriod:   20 * time.Millisecond,
		OpenTimeout:    100 * time.Millisecond,
		HalfOpenMaxReq: 2,
		WindowMinReq:   3,
		FailureRatio:   0.5,
	}

	mysqlErr := &mysql.MySQLError{Number: 1040, Message: "Too many connections"}
	genericErr := errors.New("some other error")
	canceledErr := context.Canceled
	deadlineErr := context.DeadlineExceeded

	tests := []struct {
		name     string
		sequence []error
		expected []error
	}{
		{
			name:     "all success",
			sequence: []error{nil, nil, nil, nil},
			expected: []error{nil, nil, nil, nil},
		},
		{
			name:     "open on mysql error",
			sequence: []error{nil, mysqlErr, mysqlErr, nil},
			expected: []error{nil, mysqlErr, mysqlErr, gobreaker.ErrOpenState},
		},
		{
			name:     "ignore context canceled",
			sequence: []error{canceledErr, mysqlErr, mysqlErr, nil},
			expected: []error{canceledErr, mysqlErr, mysqlErr, nil},
		},
		{
			name:     "ignore context deadline exceeded",
			sequence: []error{deadlineErr, mysqlErr, mysqlErr, nil},
			expected: []error{deadlineErr, mysqlErr, mysqlErr, nil},
		},
		{
			name:     "no open on generic error",
			sequence: []error{genericErr, genericErr, genericErr, genericErr},
			expected: []error{genericErr, genericErr, genericErr, genericErr},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := db.NewCircuitBreaker(cfg)
			ctx := context.Background()

			for i, err := range tt.sequence {
				gotErr := cb.Execute(ctx, func(ctx context.Context) error {
					return err
				})
				if !be.Err(t, gotErr, tt.expected[i]) {
					t.Logf("on i=%d", i)
				}
			}
		})
	}
}

func TestCircuitBreaker_PreservesOriginalError(t *testing.T) {
	cfg := db.CircuitBreakerConfig{
		WindowInterval: 100 * time.Millisecond,
		BucketPeriod:   20 * time.Millisecond,
		OpenTimeout:    100 * time.Millisecond,
		HalfOpenMaxReq: 2,
		WindowMinReq:   3,
		FailureRatio:   0.5,
	}
	cb := db.NewCircuitBreaker(cfg)
	ctx := context.Background()

	origErr := &mysql.MySQLError{Number: 1040, Message: "Too many connections"}

	gotErr := cb.Execute(ctx, func(ctx context.Context) error {
		return origErr
	})

	// Проверяем, что возвращена именно та же ошибка (одинаковый указатель).
	be.True(t, gotErr == origErr)
}
