package db_test

import (
	"context"
	"errors"
	"testing"

	"aaa2ppp/teams-tasks/internal/db"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/aaa2ppp/be"
)

type mockExecutor struct {
	executeCalls int
	executeFunc  func(ctx context.Context, fn func(context.Context) error) error
}

func (m *mockExecutor) Execute(ctx context.Context, fn func(context.Context) error) error {
	m.executeCalls++
	if m.executeFunc != nil {
		return m.executeFunc(ctx, fn)
	}
	return fn(ctx)
}

func TestInTxExecutor(t *testing.T) {
	tests := []struct {
		name        string
		execMock    *mockExecutor
		userErr     error
		setupDBMock func(mock sqlmock.Sqlmock)
		wantCalls   int
		wantErr     any
	}{
		{
			name: "without executor user success",
			setupDBMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectCommit()
			},
			wantCalls: 1,
		},
		{
			name:    "without executor user fail",
			userErr: errors.New("user fail"),
			setupDBMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectRollback()
			},
			wantCalls: 1,
			wantErr:   "user fail",
		},
		{
			name:     "with executor user success",
			execMock: &mockExecutor{},
			setupDBMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectCommit()
			},
			wantCalls: 1,
		},
		{
			name:     "with executor user fail",
			execMock: &mockExecutor{},
			userErr:  errors.New("user fail"),
			setupDBMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectRollback()
			},
			wantCalls: 1,
			wantErr:   "user fail",
		},
		{
			name: "with executor exec fail",
			execMock: &mockExecutor{
				executeFunc: func(ctx context.Context, fn func(context.Context) error) error {
					return errors.New("exec fail")
				},
			},
			wantCalls: 0,
			wantErr:   "exec fail",
		},
		{
			name:     "with executor begin fail",
			execMock: &mockExecutor{},
			setupDBMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(errors.New("begin fail"))
			},
			wantCalls: 0,
			wantErr:   "begin fail",
		},
		{
			name:     "with executor commit fail",
			execMock: &mockExecutor{},
			setupDBMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectCommit().WillReturnError(errors.New("commit fail"))
			},
			wantCalls: 1,
			wantErr:   "commit fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, dbMock, err := sqlmock.New()
			be.Err(t, err, nil)
			defer sqlDB.Close() //nolint:errcheck

			myDB := db.New(sqlDB)

			if tt.execMock != nil {
				myDB = myDB.WithExecutor(tt.execMock)
			}

			if tt.setupDBMock != nil {
				tt.setupDBMock(dbMock)
			}

			var calls int
			err = myDB.InTx(context.Background(), func(ctx context.Context, tx db.DBTX) error {
				calls++
				return tt.userErr
			})

			be.Err(t, err, tt.wantErr)
			be.Equal(t, calls, tt.wantCalls)

			if tt.execMock != nil {
				be.Equal(t, tt.execMock.executeCalls, 1)
			}

			be.Err(t, dbMock.ExpectationsWereMet(), nil)
		})
	}
}
