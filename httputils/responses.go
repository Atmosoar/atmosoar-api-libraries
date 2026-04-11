package httputils

import "fmt"

// ResponseServer struct to hold and json map HTTP Status, Data and Message
type ResponseServer struct {
	HTTPStatus int         `json:"httpStatus"`
	Data       interface{} `json:"data,omitempty"`
	Message    string      `json:"message,omitempty"`
}

// Response returns the ServerResponse to a string
func (res *ResponseServer) Response() string {
	return fmt.Sprintf("Server Response: %s (%v) (Http code: %d)", res.Message, res.Data, res.HTTPStatus)
}

// NewServerResponse creates a generic 200 ServerResponse
func NewServerResponse() *ResponseServer {
	return &ResponseServer{
		HTTPStatus: 200,
		Data:       "Data",
		Message:    "Data Message",
	}
}

// WithMessage replaces default message in ServerResponse
func (res *ResponseServer) WithMessage(message string) *ResponseServer {
	res.Message = message
	return res
}

// WithData replaces default data in ServerResponse
func (res *ResponseServer) WithData(data interface{}) *ResponseServer {
	res.Data = data
	return res
}

// WithHTTPStatus replaces HTTP Status Code in ServerResponse
func (res *ResponseServer) WithHTTPStatus(status int) *ResponseServer {
	res.HTTPStatus = status
	return res
}
