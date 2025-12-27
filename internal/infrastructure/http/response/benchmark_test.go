package response_test

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
)

// Small response: typical API response (~200 bytes)
type SmallResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Created string `json:"created_at"`
}

// Medium response: list of items (~2KB)
type MediumResponse struct {
	Items []SmallResponse `json:"items"`
	Total int             `json:"total"`
}

// Large response: paginated results (~20KB)
type LargeResponse struct {
	Items []MediumResponse `json:"items"`
	Page  int              `json:"page"`
	Total int              `json:"total"`
}

func createSmallResponse() SmallResponse {
	return SmallResponse{
		ID:      "01234567-89ab-cdef-0123-456789abcdef",
		Name:    "Test Item",
		Status:  "active",
		Created: "2025-12-26T15:00:00Z",
	}
}

func createMediumResponse() MediumResponse {
	items := make([]SmallResponse, 10)
	for i := range items {
		items[i] = createSmallResponse()
	}
	return MediumResponse{Items: items, Total: 10}
}

func createLargeResponse() LargeResponse {
	items := make([]MediumResponse, 10)
	for i := range items {
		items[i] = createMediumResponse()
	}
	return LargeResponse{Items: items, Page: 1, Total: 100}
}

// Benchmark json.Marshal approach (marshal first, then write)
func BenchmarkMarshalFirst_Small(b *testing.B) {
	data := createSmallResponse()
	for b.Loop() {
		w := httptest.NewRecorder()
		js, err := json.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(js)
	}
}

func BenchmarkMarshalFirst_Medium(b *testing.B) {
	data := createMediumResponse()
	for b.Loop() {
		w := httptest.NewRecorder()
		js, err := json.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(js)
	}
}

func BenchmarkMarshalFirst_Large(b *testing.B) {
	data := createLargeResponse()
	for b.Loop() {
		w := httptest.NewRecorder()
		js, err := json.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(js)
	}
}

// Benchmark json.NewEncoder approach (current implementation)
func BenchmarkEncoder_Small(b *testing.B) {
	data := createSmallResponse()
	for b.Loop() {
		w := httptest.NewRecorder()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if err := json.NewEncoder(w).Encode(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncoder_Medium(b *testing.B) {
	data := createMediumResponse()
	for b.Loop() {
		w := httptest.NewRecorder()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if err := json.NewEncoder(w).Encode(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncoder_Large(b *testing.B) {
	data := createLargeResponse()
	for b.Loop() {
		w := httptest.NewRecorder()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if err := json.NewEncoder(w).Encode(data); err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark worst case: Marshal to discard writer (no buffering benefit)
func BenchmarkMarshalFirst_Discard(b *testing.B) {
	data := createMediumResponse()
	for b.Loop() {
		js, err := json.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
		io.Discard.Write(js)
	}
}

func BenchmarkEncoder_Discard(b *testing.B) {
	data := createMediumResponse()
	for b.Loop() {
		if err := json.NewEncoder(io.Discard).Encode(data); err != nil {
			b.Fatal(err)
		}
	}
}
