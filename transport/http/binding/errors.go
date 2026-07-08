package binding

import (
	"errors"
	"net/http"
)

var (
	ErrBadRequest       = errors.New("bad request")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrNotFound         = errors.New("not found")
	ErrAlreadyExists    = errors.New("already exists")
	ErrPermissionDenied = errors.New("permission denied")
	ErrInternal         = errors.New("internal server error")
)

var errorMap = map[error]int{
	ErrBadRequest:       http.StatusBadRequest,
	ErrUnauthorized:     http.StatusUnauthorized,
	ErrNotFound:         http.StatusNotFound,
	ErrAlreadyExists:    http.StatusConflict,
	ErrPermissionDenied: http.StatusForbidden,
	ErrInternal:         http.StatusInternalServerError,
}

func MapError(err error) int {
	for sentinel, code := range errorMap {
		if errors.Is(err, sentinel) {
			return code
		}
	}
	return http.StatusInternalServerError
}

func RegisterError(err error, code int) {
	errorMap[err] = code
}