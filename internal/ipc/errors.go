package ipc

import (
	"net/http"
)

// ErrorCode 是 IPC API 的错误码，前端/日志可用于程序化排查。
type ErrorCode string

const (
	ErrorBadRequest       ErrorCode = "BAD_REQUEST"
	ErrorUnauthorized     ErrorCode = "UNAUTHORIZED"
	ErrorForbidden        ErrorCode = "FORBIDDEN"
	ErrorNotFound         ErrorCode = "NOT_FOUND"
	ErrorMethodNotAllowed ErrorCode = "METHOD_NOT_ALLOWED"
	ErrorConflict         ErrorCode = "CONFLICT"
	ErrorInternal         ErrorCode = "INTERNAL_ERROR"
	ErrorUnavailable      ErrorCode = "SERVICE_UNAVAILABLE"
)

// APIError 表示带 HTTP 状态码和错误码的 API 错误。
type APIError struct {
	Status  int       `json:"-"`
	Code    ErrorCode `json:"Code"`
	Message string    `json:"Message"`
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	return e.Message
}

// NewAPIError 创建 APIError。
func NewAPIError(status int, code ErrorCode, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

// writeErrorCode 直接写入带错误码的错误响应。
func writeErrorCode(w http.ResponseWriter, status int, code ErrorCode, message string) {
	writeJSON(w, status, map[string]any{
		"Success": false,
		"Code":    code,
		"Message": message,
	})
}
