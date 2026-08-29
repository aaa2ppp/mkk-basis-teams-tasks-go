package tasks

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aaa2ppp/teams-tasks/internal/lib/auth"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/aaa2ppp/be"
	"github.com/aaa2ppp/be/tb"
	"golang.org/x/sync/singleflight"
)

type MockStorage struct {
	Storage
	listFunc func(ctx context.Context, req DBListReq) ([]model.Task, error)
}

// List implements [Storage].
func (m *MockStorage) List(ctx context.Context, req DBListReq) ([]model.Task, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, req)
	}
	panic("unimplemented")
}

var _ Storage = &MockStorage{}

type MockCache struct {
	Cache
	getFunc func(ctx context.Context, key, field string, val any) error
	putFunc func(ctx context.Context, key, field string, val any) error
}

// Get implements [Cache].
func (m *MockCache) Get(ctx context.Context, key string, field string, val any) error {
	if m.getFunc != nil {
		return m.getFunc(ctx, key, field, val)
	}
	panic("unimplemented")
}

// Put implements [Cache].
func (m *MockCache) Put(ctx context.Context, key string, field string, val any) error {
	if m.putFunc != nil {
		return m.putFunc(ctx, key, field, val)
	}
	panic("unimplemented")
}

var _ Cache = &MockCache{}

func TestService_List_SingleflightCoalescesCacheMisses(t *testing.T) {

	tests := []struct {
		name    string
		want    []model.Task
		wantErr error
	}{
		{
			name: "success",
			want: []model.Task{{ID: 1, TeamID: 123, Title: "task1"}},
		},
		{
			name:    "failed",
			want:    nil,
			wantErr: errors.New("some error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var storListCount int32
			var cacheGetCount int32
			var cachePutCount int32

			mockStorage := &MockStorage{
				listFunc: func(ctx context.Context, req DBListReq) ([]model.Task, error) {
					time.Sleep(10 * time.Millisecond)
					atomic.AddInt32(&storListCount, 1)
					return tt.want, tt.wantErr
				},
			}

			mockCache := &MockCache{
				getFunc: func(ctx context.Context, key, field string, val any) error {
					time.Sleep(10 * time.Millisecond)
					atomic.AddInt32(&cacheGetCount, 1)
					return model.ErrNotFound
				},
				putFunc: func(ctx context.Context, key, field string, val any) error {
					time.Sleep(10 * time.Millisecond)
					atomic.AddInt32(&cachePutCount, 1)
					return nil
				},
			}

			svc := NewService(mockStorage, nil, mockCache)
			svc.sf = &singleflight.Group{}

			req := SvcListReq{
				TeamID: 123,
				Status: model.StatusTodo,
				Limit:  len(tt.want),
			}

			const concurrency = 20

			// Запускаем множество конкурентных вызовов
			var wg sync.WaitGroup
			wg.Add(concurrency)

			type result struct {
				tasks []model.Task
				err   error
			}
			resCh := make(chan result, concurrency)

			for i := 1; i <= concurrency; i++ {
				go func() {
					defer wg.Done()
					user := model.User{
						ID: model.UserID(i),
						Roles: map[string]model.Role{
							"123": model.RoleMember,
						},
					}
					ctx := auth.ContextWithUserForTest(context.Background(), user)
					resp, err := svc.List(ctx, req)
					resCh <- result{resp.Tasks, err}
				}()
			}
			wg.Wait()
			close(resCh)

			be.Equal(t, storListCount, 1)
			be.Equal(t, cacheGetCount, 1)
			if tt.wantErr != nil {
				be.Equal(t, cachePutCount, 0)
			} else {
				be.Equal(t, cachePutCount, 1)
			}

			for res := range resCh {
				if !be.Err(t, res.err, tt.wantErr) {
					break
				}
				if !be.Equal(tb.Diff(t), res.tasks, tt.want) {
					break
				}
			}
		})
	}
}
