package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCoreEndpoints(t *testing.T) {
	handler := New()
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/health", 200},
		{"/api/config", 200},
		{"/api/ping", 204},
		{"/api/download?bytes=4096", 200},
		{"/api/download?bytes=0", 400},
		{"/api/ip", 200},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Errorf("%s: got %d, want %d", tc.path, res.Code, tc.status)
		}
	}
}

func TestDownloadSizeAndHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/download?bytes=1000", nil)
	res := httptest.NewRecorder()
	New().ServeHTTP(res, req)
	if got := len(res.Body.Bytes()); got != 1000 {
		t.Fatalf("received %d bytes, want 1000", got)
	}
	if res.Header().Get("Cache-Control") != "no-store, no-transform" {
		t.Fatal("missing no-store response header")
	}
}

func TestUpload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/upload", bytes.NewReader(make([]byte, 1024)))
	res := httptest.NewRecorder()
	New().ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("upload status: %d", res.Code)
	}
	var response struct {
		ReceivedBytes int64 `json:"receivedBytes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ReceivedBytes != 1024 {
		t.Fatalf("received %d bytes", response.ReceivedBytes)
	}
}

func TestRejectLargeUpload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/upload", bytes.NewReader(make([]byte, maxUpload+1)))
	res := httptest.NewRecorder()
	New().ServeHTTP(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", res.Code)
	}
}

func TestMethodValidation(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/ping", nil)
	res := httptest.NewRecorder()
	New().ServeHTTP(res, req)
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d, want 405", res.Code)
	}
}
