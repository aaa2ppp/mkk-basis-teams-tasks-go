package db

import "context"

// Executor базовый интерфейс для выполнения методов. Позволяет встраивать логи, ретраи, брейкеры etc
type Executor interface {
	Execute(ctx context.Context, fn func(context.Context) error) error
}

// Execute обертка для методов, возвращающих результат и ошибку: SomeMethod(ctx, Req) (Resp, error)
func Execute[Req any, Resp any](
	exec Executor,
	ctx context.Context,
	req Req,
	fn func(context.Context, Req) (Resp, error),
) (Resp, error) {
	var resp Resp
	err := exec.Execute(ctx, func(c context.Context) error {
		var err error
		resp, err = fn(c, req)
		return err
	})
	return resp, err
}

// ExecuteErr обертка для методов, возвращающих только ошибку: SomeMethod(ctx, Req) error
func ExecuteErr[Req any](
	exec Executor,
	ctx context.Context,
	req Req,
	fn func(context.Context, Req) error,
) error {
	return exec.Execute(ctx, func(c context.Context) error {
		return fn(c, req)
	})
}
