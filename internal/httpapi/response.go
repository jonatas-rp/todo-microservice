package httpapi

import "net/http"

// envelope is the body of every successful response.
type envelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func writeSuccess(w http.ResponseWriter, status int, message string, data any) {
	writeJSON(w, status, contentTypeJSON, envelope{Success: true, Message: message, Data: data})
}
