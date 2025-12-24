package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// OK sends a 200 OK response with JSON data.
func OK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode success response", "error", err)
	}
}

// Created sends a 201 Created response with JSON data.
func Created(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode created response", "error", err)
	}
}

// NoContent sends a 204 No Content response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
