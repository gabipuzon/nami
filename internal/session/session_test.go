package session

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabipuzon/nami/internal/storage"
)

func request(s *Session, method, path, body, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:7331"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestRescanKeepsOldReadsAndRejectsConcurrentWrites(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	s := New(storage.Snapshot{ID: "old"}, func() (storage.Snapshot, error) { close(entered); <-finish; return storage.Snapshot{ID: "new"}, nil })
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- request(s, "POST", "/api/v1/rescan", `{"snapshot_id":"old"}`, "") }()
	<-entered
	if w := request(s, "GET", "/api/v1/scan", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"old"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(s, "POST", "/api/v1/rescan", `{"snapshot_id":"old"}`, ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	close(finish)
	if w := <-done; w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(s, "GET", "/api/v1/scan", "", ""); !strings.Contains(w.Body.String(), `"id":"new"`) {
		t.Fatal(w.Body.String())
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:7331/api/v1/graph", nil)
	r.Header.Set("X-Nami-Snapshot", "old")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestFailureAndOriginPreserveSnapshot(t *testing.T) {
	calls := 0
	s := New(storage.Snapshot{ID: "old"}, func() (storage.Snapshot, error) { calls++; return storage.Snapshot{}, errors.New("bad ignore") })
	if w := request(s, "POST", "/api/v1/rescan", `{"snapshot_id":"old"}`, "http://example.com"); w.Code != 403 || calls != 0 {
		t.Fatal(w.Code, calls)
	}
	if w := request(s, "POST", "/api/v1/rescan", `{"snapshot_id":"old"}`, "http://127.0.0.1:7331"); w.Code != 500 {
		t.Fatal(w.Code)
	}
	if w := request(s, "GET", "/api/v1/scan", "", ""); !strings.Contains(w.Body.String(), `"id":"old"`) {
		t.Fatal(w.Body.String())
	}
	if w := request(s, "POST", "/api/v1/rescan", `{"snapshot_id":"old"}`, ""); w.Code != 500 || calls != 2 {
		t.Fatal(w.Code, calls)
	}
}

func TestDevelopmentProxyOriginAndJSONValidation(t *testing.T) {
	calls := 0
	s := New(storage.Snapshot{ID: "old"}, func() (storage.Snapshot, error) { calls++; return storage.Snapshot{ID: "new"}, nil })
	r := httptest.NewRequest("POST", "http://127.0.0.1:7331/api/v1/rescan", strings.NewReader(`{"snapshot_id":"old"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost:3000")
	r.Header.Set("X-Forwarded-Host", "localhost:3000")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 || calls != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	r = httptest.NewRequest("POST", "http://evil.test:7331/api/v1/rescan", strings.NewReader(`{"snapshot_id":"new"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://evil.test:7331")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 || calls != 1 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/api/v1/rescan", `{"snapshot_id":"new","extra":true}`, ""); w.Code != 400 || calls != 1 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/api/v1/rescan", `{"snapshot_id":"old"}`, ""); w.Code != 409 || calls != 1 {
		t.Fatal(w.Code)
	}
	if w := request(s, "GET", "/api/v1/rescan", "", ""); w.Code != 405 {
		t.Fatal(w.Code)
	}
}
