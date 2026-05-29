package platform

import (
	"encoding/json"
	"net/http"
)

type ErrorDetail struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Detail     string `json:"detail,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	Meta       any    `json:"meta,omitempty"`
}

type Response struct {
	Success bool         `json:"success"`
	Data    any          `json:"data,omitempty"`
	Error   *ErrorDetail `json:"error,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{Success: true, Data: data})
}

func WriteError(w http.ResponseWriter, status int, code, message, detail, suggestion string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{Success: false, Error: &ErrorDetail{Code: code, Message: message, Detail: detail, Suggestion: suggestion}})
}

func WriteErrorWithMeta(w http.ResponseWriter, status int, code, message, detail, suggestion string, meta any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{Success: false, Error: &ErrorDetail{Code: code, Message: message, Detail: detail, Suggestion: suggestion, Meta: meta}})
}
