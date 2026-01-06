package models

import (
	"fmt"
	"net/http"
)

const (
	NotFoundErrorCode       = http.StatusNotFound
	InternalServerErrorCode = http.StatusInternalServerError
	BadRequestErrorCode     = http.StatusBadRequest
	UnauthorizedErrorCode   = http.StatusUnauthorized
	StorageExpiredCode      = http.StatusGone
	ServiceUnavailableCode  = http.StatusServiceUnavailable
)

var defaultMessages = map[int]string{
	InternalServerErrorCode: "internal server error",
	BadRequestErrorCode:     "bad request",
	NotFoundErrorCode:       "not found",
}

// AppError — custom error type to handle service layer errors
type AppError struct {
	Code    int
	Message string
}

func (e *AppError) Error() string {
	return fmt.Sprintf("code=%d, message=%s", e.Code, e.Message)
}

func NewAppError(code int, message string) *AppError {
	if message == "" {
		if defMsg, ok := defaultMessages[code]; ok {
			message = defMsg
		} else {
			message = "error"
		}
	}
	return &AppError{
		Code:    code,
		Message: message,
	}
}
