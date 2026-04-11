package httputils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewServerResponse tests the default server response creation
func TestNewServerResponse(t *testing.T) {
	res := NewServerResponse()

	assert.Equal(t, 200, res.HTTPStatus)
	assert.Equal(t, "Data", res.Data)
	assert.Equal(t, "Data Message", res.Message)
}

// TestWithMessage tests the WithMessage method
func TestResponseWithMessage(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{"simple message", "Success response", "Success response"},
		{"empty message", "", ""},
		{
			"long message",
			"This is a very long success message describing the operation in detail",
			"This is a very long success message describing the operation in detail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := NewServerResponse().WithMessage(tt.message)
			assert.Equal(t, tt.expected, res.Message)
			assert.Equal(t, 200, res.HTTPStatus) // Status should remain unchanged
			assert.Equal(t, "Data", res.Data)    // Data should remain unchanged
		})
	}
}

// TestWithData tests the WithData method
func TestWithData(t *testing.T) {
	tests := []struct {
		name     string
		data     interface{}
		expected interface{}
	}{
		{"string data", "test data", "test data"},
		{"int data", 42, 42},
		{"struct data", struct{ Field string }{Field: "value"}, struct{ Field string }{Field: "value"}},
		{"nil data", nil, nil},
		{"map data", map[string]int{"a": 1, "b": 2}, map[string]int{"a": 1, "b": 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := NewServerResponse().WithData(tt.data)
			assert.Equal(t, tt.expected, res.Data)
			assert.Equal(t, 200, res.HTTPStatus)         // Status should remain unchanged
			assert.Equal(t, "Data Message", res.Message) // Message should remain unchanged
		})
	}
}

// TestWithHTTPStatus tests the WithHTTPStatus method
func TestResponseWithHTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		expected int
	}{
		{"created", 201, 201},
		{"accepted", 202, 202},
		{"no content", 204, 204},
		{"bad request", 400, 400},
		{"not found", 404, 404},
		{"internal server error", 500, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := NewServerResponse().WithHTTPStatus(tt.status)
			assert.Equal(t, tt.expected, res.HTTPStatus)
			assert.Equal(t, "Data Message", res.Message) // Message should remain unchanged
			assert.Equal(t, "Data", res.Data)            // Data should remain unchanged
		})
	}
}

// TestResponseMethod tests the Response() method implementation
func TestResponseMethod(t *testing.T) {
	tests := []struct {
		name     string
		res      *ResponseServer
		expected string
	}{
		{
			"default response",
			NewServerResponse(),
			"Server Response: Data Message (Data) (Http code: 200)",
		},
		{
			"custom message",
			NewServerResponse().WithMessage("Operation successful"),
			"Server Response: Operation successful (Data) (Http code: 200)",
		},
		{
			"custom data",
			NewServerResponse().WithData(42),
			"Server Response: Data Message (42) (Http code: 200)",
		},
		{
			"custom status",
			NewServerResponse().WithHTTPStatus(201),
			"Server Response: Data Message (Data) (Http code: 201)",
		},
		{
			"complex data",
			NewServerResponse().WithData(struct{ ID int }{ID: 123}),
			"Server Response: Data Message ({123}) (Http code: 200)",
		},
		{
			"empty fields",
			&ResponseServer{},
			"Server Response:  (<nil>) (Http code: 0)",
		},
		{
			"nil data",
			NewServerResponse().WithData(nil),
			"Server Response: Data Message (<nil>) (Http code: 200)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.res.Response())
		})
	}
}

// TestChaining tests method chaining
func TestChaining(t *testing.T) {
	res := NewServerResponse().
		WithMessage("Resource created").
		WithData(map[string]string{"id": "123"}).
		WithHTTPStatus(201)

	assert.Equal(t, 201, res.HTTPStatus)
	assert.Equal(t, "Resource created", res.Message)
	assert.Equal(t, map[string]string{"id": "123"}, res.Data)

	// Verify the string representation
	assert.Equal(t,
		"Server Response: Resource created (map[id:123]) (Http code: 201)",
		res.Response(),
	)
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	t.Run("zero values", func(t *testing.T) {
		res := &ResponseServer{}
		assert.Equal(t, 0, res.HTTPStatus)
		assert.Empty(t, res.Message)
		assert.Nil(t, res.Data)
		assert.Equal(t, "Server Response:  (<nil>) (Http code: 0)", res.Response())
	})

	t.Run("negative status code", func(t *testing.T) {
		res := NewServerResponse().WithHTTPStatus(-1)
		assert.Equal(t, -1, res.HTTPStatus)
		assert.Equal(t,
			"Server Response: Data Message (Data) (Http code: -1)",
			res.Response(),
		)
	})

	t.Run("very long strings", func(t *testing.T) {
		longMsg := "This is an extremely long success message that provides detailed " +
			"information about the successful operation. It includes multiple " +
			"sentences and might even contain additional context or metadata."

		complexData := struct {
			ID      string
			Details string
		}{
			ID:      "12345-67890-abcde-fghij",
			Details: "This is a complex data structure with multiple fields",
		}

		res := NewServerResponse().
			WithMessage(longMsg).
			WithData(complexData).
			WithHTTPStatus(200)

		assert.Equal(t, longMsg, res.Message)
		assert.Equal(t, complexData, res.Data)
		assert.Equal(t, 200, res.HTTPStatus)

		expectedResponse := fmt.Sprintf(
			"Server Response: %s ({12345-67890-abcde-fghij This is a complex data structure with multiple fields}) (Http code: 200)",
			longMsg,
		)
		assert.Equal(t, expectedResponse, res.Response())
	})
}

// TestResponseMethodWithSpecialCharacters tests special characters in messages
func TestResponseMethodWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		data     interface{}
		expected string
	}{
		{"quotes", `Message with "quotes"`, "data", `Server Response: Message with "quotes" (data) (Http code: 200)`},
		{"newline", "Message with\nnewline", 42, "Server Response: Message with\nnewline (42) (Http code: 200)"},
		{
			"unicode",
			"Message with unicode: 日本語",
			true,
			"Server Response: Message with unicode: 日本語 (true) (Http code: 200)",
		},
		{
			"backslashes",
			`Message with \\ backslashes`,
			nil,
			`Server Response: Message with \\ backslashes (<nil>) (Http code: 200)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := NewServerResponse().
				WithMessage(tt.message).
				WithData(tt.data)

			assert.Equal(t, tt.expected, res.Response())
		})
	}
}
