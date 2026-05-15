package httputils

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testLogger() *zap.SugaredLogger {
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

func setupTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	return c, w
}

// --- APIError struct tests ---

func TestAPIError_Error(t *testing.T) {
	e := &APIError{
		HTTPStatus: 400,
		Code:       CodeValidationError,
		Message:    "Invalid input",
	}
	assert.Equal(t, "VALIDATION_ERROR: Invalid input (HTTP 400)", e.Error())
}

func TestAPIError_JSON(t *testing.T) {
	e := APIError{
		HTTPStatus: 400,
		Code:       CodeInvalidLocation,
		Message:    "Location parameter cannot be parsed.",
		Errors: []FieldError{
			{Code: CodeInvalidLocation, Field: "location", Message: "Expected lat,lon format."},
		},
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, float64(400), decoded["httpStatus"])
	assert.Equal(t, "INVALID_LOCATION", decoded["code"])
	assert.Equal(t, "Location parameter cannot be parsed.", decoded["message"])

	errs, ok := decoded["errors"].([]interface{})
	require.True(t, ok)
	assert.Len(t, errs, 1)
}

func TestAPIError_JSON_OmitsEmptyErrors(t *testing.T) {
	e := APIError{
		HTTPStatus: 401,
		Code:       CodeAuthFailed,
		Message:    "Authentication failed.",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))

	_, exists := decoded["errors"]
	assert.False(t, exists, "errors array should be omitted when empty")
}

// --- ValidationError tests ---

func TestValidationError_Error(t *testing.T) {
	ve := &ValidationError{
		Fields: []FieldError{
			{Code: CodeInvalidLocation, Field: "location", Message: "invalid format"},
			{Code: CodeInvalidTimeFormat, Field: "time", Message: "expected RFC3339"},
		},
	}
	assert.Equal(t, "location: invalid format; time: expected RFC3339", ve.Error())
}

func TestValidationError_ErrorNoField(t *testing.T) {
	ve := &ValidationError{
		Fields: []FieldError{
			{Code: CodeMissingRequired, Message: "parameter is required"},
		},
	}
	assert.Equal(t, "parameter is required", ve.Error())
}

func TestValidationError_ErrorsAs(t *testing.T) {
	ve := &ValidationError{
		Fields: []FieldError{
			{Code: CodeInvalidLocation, Field: "location", Message: "invalid"},
		},
	}

	var target *ValidationError
	assert.True(t, errors.As(ve, &target))
	assert.Len(t, target.Fields, 1)
}

// --- SendAPIError tests ---

func TestSendAPIError_400(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	SendAPIError(c, logger, 400, CodeInvalidLocation, "Location invalid",
		FieldError{Code: CodeInvalidLocation, Field: "location", Message: "bad format"})

	assert.Equal(t, 400, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeInvalidLocation, resp.Code)
	assert.Equal(t, "Location invalid", resp.Message)
	assert.Len(t, resp.Errors, 1)
}

func TestSendAPIError_500(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	SendAPIError(c, logger, 500, CodeInternalError, "Unexpected error")

	assert.Equal(t, 500, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeInternalError, resp.Code)
	assert.Empty(t, resp.Errors)
}

// --- SendValidationError tests ---

func TestSendValidationError_Multiple(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	fields := []FieldError{
		{Code: CodeInvalidLocation, Field: "location", Message: "invalid format"},
		{Code: CodeInvalidTimeFormat, Field: "time", Message: "expected RFC3339"},
	}

	SendValidationError(c, logger, fields)

	assert.Equal(t, 400, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeValidationError, resp.Code)
	assert.Equal(t, "2 request parameters failed validation. See errors[].", resp.Message)
	assert.Len(t, resp.Errors, 2)
}

func TestSendValidationError_Single(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	fields := []FieldError{
		{Code: CodeInvalidLocation, Field: "location", Message: "Expected lat,lon format."},
	}

	SendValidationError(c, logger, fields)

	assert.Equal(t, 400, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// Single-field case now uses the same summary shape as multi-field.
	assert.Equal(t, CodeValidationError, resp.Code)
	assert.Equal(t, "1 request parameter failed validation. See errors[].", resp.Message)
	require.Len(t, resp.Errors, 1)
	assert.Equal(t, CodeInvalidLocation, resp.Errors[0].Code)
	assert.Equal(t, "location", resp.Errors[0].Field)
	assert.Equal(t, "Expected lat,lon format.", resp.Errors[0].Message)
}

// --- Convenience helper tests ---

func TestSendAuthError(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	SendAuthError(c, logger, errors.New("token issuer mismatch: https://keycloak.internal"))

	assert.Equal(t, 401, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeAuthFailed, resp.Code)
	assert.Equal(t, "Authentication failed. Please provide a valid access token.", resp.Message)
	// AC-10: No internal details leaked
	assert.NotContains(t, w.Body.String(), "keycloak")
	assert.Empty(t, resp.Errors)
}

func TestSendQuotaError(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	SendQuotaError(c, logger, errors.New("Tier: premium/5000"))

	assert.Equal(t, 429, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeQuotaExceeded, resp.Code)
	assert.Equal(t, "API quota exceeded. Please try again later.", resp.Message)
	// AC-12: No tier details leaked
	assert.NotContains(t, w.Body.String(), "premium")
	assert.NotContains(t, w.Body.String(), "5000")
}

func TestSendUpstreamError(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	SendUpstreamError(
		c,
		logger,
		"Weather Data Ingestor",
		errors.New("gRPC UNAVAILABLE: connection refused"),
	)

	assert.Equal(t, 502, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeUpstreamError, resp.Code)
	// Top-level message no longer carries the source name — it lives in errors[].
	assert.Equal(t, "Upstream data source unavailable.", resp.Message)
	// AC-14: No gRPC codes or hostnames
	assert.NotContains(t, w.Body.String(), "gRPC")
	assert.NotContains(t, w.Body.String(), "connection refused")
	require.Len(t, resp.Errors, 1)
	assert.Equal(t, CodeDataSourceUnavail, resp.Errors[0].Code)
	assert.Equal(t, "weather_data_ingestor", resp.Errors[0].Field)
	assert.Equal(
		t,
		"Weather Data Ingestor is temporarily unavailable; please retry.",
		resp.Errors[0].Message,
	)
}

func TestSendInternalError(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	SendInternalError(c, logger, errors.New("nil pointer dereference in forecastReader.go:42"))

	assert.Equal(t, 500, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeInternalError, resp.Code)
	assert.Equal(t, "An unexpected error occurred. Please try again later.", resp.Message)
	// AC-16: No internal details
	assert.NotContains(t, w.Body.String(), "nil pointer")
	assert.NotContains(t, w.Body.String(), "forecastReader")
}

func TestSendInternalError_WithRequestID(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()
	c.Set("X-Request-Id", "req-abc-123")

	SendInternalError(c, logger, errors.New("nil pointer dereference in forecastReader.go:42"))

	assert.Equal(t, 500, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeInternalError, resp.Code)
	assert.Equal(
		t,
		"An unexpected error occurred. Reference request ID req-abc-123 when contacting support.",
		resp.Message,
	)
	// AC-16: No internal details
	assert.NotContains(t, w.Body.String(), "nil pointer")
	assert.NotContains(t, w.Body.String(), "forecastReader")
}

func TestSendNotFound(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	SendNotFound(c, logger)

	assert.Equal(t, 404, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeNotFound, resp.Code)
	assert.Equal(t, "The requested endpoint does not exist.", resp.Message)
}

func TestSendForbidden(t *testing.T) {
	c, w := setupTestContext()
	logger := testLogger()

	SendForbidden(c, logger, errors.New("pilot role cannot list users"))

	assert.Equal(t, 403, w.Code)

	var resp APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeForbidden, resp.Code)
	assert.NotContains(t, w.Body.String(), "pilot")
}

// --- Error code constants test ---

func TestErrorCodes(t *testing.T) {
	codes := []string{
		CodeValidationError, CodeInvalidLocation, CodeInvalidTimeFormat,
		CodeInvalidTimeRange, CodeInvalidParameter, CodeInvalidOutput,
		CodeInvalidThreshold, CodeMissingRequired, CodeInvalidPagination,
		CodeAuthFailed, CodeForbidden, CodeNotFound, CodeQuotaExceeded,
		CodeInternalError, CodeUpstreamError, CodeDataSourceUnavail,
	}

	seen := make(map[string]bool)
	for _, code := range codes {
		assert.NotEmpty(t, code, "error code must not be empty")
		assert.False(t, seen[code], "duplicate error code: %s", code)
		seen[code] = true
	}
}
