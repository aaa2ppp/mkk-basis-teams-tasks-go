package api

import (
	"net/http"
)

type Service interface {
}

func New(svc Service) *http.ServeMux {
	mux := http.NewServeMux()
	return mux
}
