package response

import (
	"encoding/json"
	"net/http"
)

// OK sends a 200 OK response with JSON data.
// If JSON marshaling fails, sends 500 Internal Server Error with standard error format.
func OK(w http.ResponseWriter, data interface{}) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		writeMarshalingError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jsonBytes)
}

// Created sends a 201 Created response with JSON data.
// If JSON marshaling fails, sends 500 Internal Server Error with standard error format.
func Created(w http.ResponseWriter, data interface{}) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		writeMarshalingError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(jsonBytes)
}

// NoContent sends a 204 No Content response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
