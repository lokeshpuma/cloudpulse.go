package response

import (
	"encoding/json"
	"net/http"
)

type Envelope struct {
	Success   bool       `json:"success"`
	Data      any        `json:"data,omitempty"`
	Message   string     `json:"message,omitempty"`
	Error     *ErrorBody `json:"error,omitempty"`
	RequestID string     `json:"request_id,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Paginated struct {
	Items      any `json:"items"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}

func JSON(w http.ResponseWriter, r *http.Request, status int, data any, message string) {
	reqID := w.Header().Get("X-Request-ID")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Success:   true,
		Data:      data,
		Message:   message,
		RequestID: reqID,
	})
}

func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reqID := w.Header().Get("X-Request-ID")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
		},
		RequestID: reqID,
	})
}
