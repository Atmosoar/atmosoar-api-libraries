// Package httputils holds unified error and success response types for
// Atmosoar HTTP services. It provides the structured error envelope
// (introduced in MMA-165) and typed error-code constants so every service
// emits identical error shapes.
package httputils

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// --- Error Code Catalog ---
// AC-2: Stable, uppercase, underscore-separated error codes.

// Error codes — stable, machine-readable identifiers for API consumers.
const (
	CodeValidationError   = "VALIDATION_ERROR"
	CodeInvalidLocation   = "INVALID_LOCATION"
	CodeInvalidTimeFormat = "INVALID_TIME_FORMAT"
	CodeInvalidTimeRange  = "INVALID_TIME_RANGE"
	CodeInvalidParameter  = "INVALID_PARAMETER"
	CodeInvalidOutput     = "INVALID_OUTPUT_FORMAT"
	CodeInvalidThreshold  = "INVALID_THRESHOLD"
	CodeMissingRequired   = "MISSING_REQUIRED_FIELD"
	CodeInvalidPagination = "INVALID_PAGINATION"
	CodeAuthFailed        = "AUTHENTICATION_FAILED"
	CodeForbidden         = "FORBIDDEN"
	CodeNotFound          = "NOT_FOUND"
	CodeQuotaExceeded     = "QUOTA_EXCEEDED"
	CodeInternalError     = "INTERNAL_ERROR"
	CodeUpstreamError     = "UPSTREAM_ERROR"
	CodeDataSourceUnavail = "DATA_SOURCE_UNAVAILABLE"

	// Cell-level error codes — surfaced per cell in forecast responses (MMA-167).
	CodeCellMissingData         = "MISSING_DATA"
	CodeCellFetchFailed         = "FETCH_FAILED"
	CodeCellGRIBReadFailed      = "GRIB_READ_FAILED"
	CodeCellDerivedNotSupported = "DERIVED_NOT_SUPPORTED"
	CodeCellDataError           = "DATA_ERROR"
)

// --- Structured Error Types ---

// FieldError represents a single field-level validation issue.
// AC-5: Each entry includes code, field (if applicable), and message.
type FieldError struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// APIError is the unified error response envelope.
// AC-1: All error responses use this structure.
type APIError struct {
	HTTPStatus int          `json:"httpStatus"`
	Code       string       `json:"code"`
	Message    string       `json:"message"`
	Errors     []FieldError `json:"errors,omitempty"`
}

// Error implements the error interface so APIError can be used as an error value.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Message, e.HTTPStatus)
}

// ValidationError wraps a slice of FieldErrors and implements the error interface.
// Handlers can type-assert via errors.As to extract structured field details.
type ValidationError struct {
	Fields []FieldError
}

// Error returns a combined message from all field errors.
func (v *ValidationError) Error() string {
	msgs := make([]string, len(v.Fields))
	for i, f := range v.Fields {
		if f.Field != "" {
			msgs[i] = fmt.Sprintf("%s: %s", f.Field, f.Message)
		} else {
			msgs[i] = f.Message
		}
	}
	return strings.Join(msgs, "; ")
}

// --- Builder Functions ---

// SendAPIError writes a structured JSON error response and logs it via Zap.
// AC-17: Every error response is accompanied by a structured log entry.
// AC-18: 4xx → Warnw, 5xx → Errorw.
func SendAPIError(
	c *gin.Context,
	logger *zap.SugaredLogger,
	status int,
	code, message string,
	fields ...FieldError,
) {
	resp := APIError{
		HTTPStatus: status,
		Code:       code,
		Message:    message,
	}
	if len(fields) > 0 {
		resp.Errors = fields
	}

	// AC-17/AC-18: Structured log with request context.
	logFields := []interface{}{
		"errorCode", code,
		"httpStatus", status,
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"clientIP", c.ClientIP(),
	}

	if status >= 500 {
		logger.Errorw("Error response", logFields...)
	} else {
		logger.Warnw("Error response", logFields...)
	}

	c.AbortWithStatusJSON(status, resp)
}

// SendValidationError writes a 400 response with one envelope shape regardless of field count.
// Top-level always carries VALIDATION_ERROR + a summary; per-field detail lives in errors[].
func SendValidationError(c *gin.Context, logger *zap.SugaredLogger, fields []FieldError) {
	noun := "parameters"
	if len(fields) == 1 {
		noun = "parameter"
	}
	msg := fmt.Sprintf("%d request %s failed validation. See errors[].", len(fields), noun)

	SendAPIError(c, logger, 400, CodeValidationError, msg, fields...)
}

// --- Convenience helpers for common error patterns ---

// SendAuthError sends a generic 401 with no internal details.
// AC-9/AC-10: All auth failures return the same response.
func SendAuthError(c *gin.Context, logger *zap.SugaredLogger, internalErr error) {
	if logger != nil && internalErr != nil {
		logger.Warnw("Authentication failed",
			"error", internalErr.Error(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"clientIP", c.ClientIP(),
		)
	}
	c.AbortWithStatusJSON(401, APIError{
		HTTPStatus: 401,
		Code:       CodeAuthFailed,
		Message:    "Authentication failed. Please provide a valid access token.",
	})
}

// SendQuotaError sends a generic 429 with no internal details.
// AC-11/AC-12: No tier names, limits, or Redis details.
func SendQuotaError(c *gin.Context, logger *zap.SugaredLogger, internalErr error) {
	if logger != nil && internalErr != nil {
		logger.Warnw("Quota exceeded",
			"error", internalErr.Error(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"clientIP", c.ClientIP(),
		)
	}
	c.AbortWithStatusJSON(429, APIError{
		HTTPStatus: 429,
		Code:       CodeQuotaExceeded,
		Message:    "API quota exceeded. Please try again later.",
	})
}

// SendUpstreamError sends a 502 naming the data source but hiding internals.
// AC-13/AC-14: Names the source but no gRPC codes, HTTP codes, or hostnames.
func SendUpstreamError(
	c *gin.Context,
	logger *zap.SugaredLogger,
	sourceName string,
	internalErr error,
) {
	if logger != nil && internalErr != nil {
		logger.Errorw("Upstream data source error",
			"source", sourceName,
			"error", internalErr.Error(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"clientIP", c.ClientIP(),
		)
	}

	field := FieldError{
		Code:    CodeDataSourceUnavail,
		Field:   strings.ToLower(strings.ReplaceAll(sourceName, " ", "_")),
		Message: fmt.Sprintf("%s is temporarily unavailable; please retry.", sourceName),
	}

	c.AbortWithStatusJSON(502, APIError{
		HTTPStatus: 502,
		Code:       CodeUpstreamError,
		Message:    "Upstream data source unavailable.",
		Errors:     []FieldError{field},
	})
}

// SendInternalError sends a generic 500 with no internal details.
// AC-15/AC-16: No stack traces, panic details, or function names.
// If upstream middleware has set the "X-Request-Id" key on the gin.Context,
// the id is interpolated into the user-facing message and added to the log line.
func SendInternalError(c *gin.Context, logger *zap.SugaredLogger, internalErr error) {
	requestID, _ := c.Get("X-Request-Id")
	requestIDStr, _ := requestID.(string)

	if logger != nil && internalErr != nil {
		fields := []interface{}{
			"error", internalErr.Error(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"clientIP", c.ClientIP(),
		}
		if requestIDStr != "" {
			fields = append(fields, "requestId", requestIDStr)
		}
		logger.Errorw("Internal server error", fields...)
	}

	msg := "An unexpected error occurred. Please try again later."
	if requestIDStr != "" {
		msg = fmt.Sprintf(
			"An unexpected error occurred. Reference request ID %s when contacting support.",
			requestIDStr,
		)
	}

	c.AbortWithStatusJSON(500, APIError{
		HTTPStatus: 500,
		Code:       CodeInternalError,
		Message:    msg,
	})
}

// SendNotFound sends a 404 for unknown endpoints.
func SendNotFound(c *gin.Context, logger *zap.SugaredLogger) {
	if logger != nil {
		logger.Warnw("Not found",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"clientIP", c.ClientIP(),
		)
	}
	c.AbortWithStatusJSON(404, APIError{
		HTTPStatus: 404,
		Code:       CodeNotFound,
		Message:    "The requested endpoint does not exist.",
	})
}

// SendForbidden sends a 403 for insufficient permissions.
func SendForbidden(c *gin.Context, logger *zap.SugaredLogger, internalErr error) {
	if logger != nil && internalErr != nil {
		logger.Warnw("Forbidden",
			"error", internalErr.Error(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"clientIP", c.ClientIP(),
		)
	}
	c.AbortWithStatusJSON(403, APIError{
		HTTPStatus: 403,
		Code:       CodeForbidden,
		Message:    "Insufficient permissions to access this resource.",
	})
}
