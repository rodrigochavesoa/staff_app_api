package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func parseIDParam(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}
