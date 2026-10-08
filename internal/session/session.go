// Package session owns writes and snapshot replacement; graph handlers stay immutable.
package session

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/api"
	"github.com/gabipuzon/nami/internal/storage"
)

type Scan func() (storage.Snapshot, error)
type Session struct {
	mu       sync.Mutex
	active   storage.Snapshot
	handler  http.Handler
	scanning bool
	scan     Scan
}

func New(snapshot storage.Snapshot, scan Scan) *Session {
	return &Session{active: snapshot, handler: api.NewHandler(snapshot), scan: scan}
}

func RepositoryScan(root string) Scan {
	return func() (storage.Snapshot, error) {
		result, err := analysis.Map(root)
		if err != nil {
			return storage.Snapshot{}, err
		}
		store, err := storage.Open(root)
		if err != nil {
			return storage.Snapshot{}, err
		}
		summary, saveErr := store.Save(root, result)
		closeErr := store.Close()
		if saveErr != nil {
			return storage.Snapshot{}, saveErr
		}
		if closeErr != nil {
			return storage.Snapshot{}, closeErr
		}
		return storage.Snapshot{ID: summary.ID, Root: summary.Root, CreatedAt: summary.CreatedAt, Status: summary.Status, Result: result}, nil
	}
}

func fail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func (s *Session) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !localHost(r.Host) {
		fail(w, 403, "forbidden_host", "only localhost requests are allowed")
		return
	}
	if r.URL.Path != "/api/v1/rescan" {
		s.mu.Lock()
		handler := s.handler
		s.mu.Unlock()
		handler.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		fail(w, 405, "method_not_allowed", "only POST is allowed")
		return
	}
	host := r.Host
	// Next development forwards its same-origin browser host through the loopback proxy.
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" && localHost(forwarded) {
		host = forwarded
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+host {
		fail(w, 403, "forbidden_origin", "cross-origin rescan is not allowed")
		return
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		fail(w, 403, "forbidden_origin", "cross-origin rescan is not allowed")
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		fail(w, 415, "bad_request", "application/json is required")
		return
	}
	var input struct {
		SnapshotID string `json:"snapshot_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.SnapshotID == "" {
		fail(w, 400, "bad_request", "snapshot_id is required")
		return
	}
	s.mu.Lock()
	if s.scanning || input.SnapshotID != s.active.ID {
		s.mu.Unlock()
		fail(w, 409, "scan_conflict", "a scan is running or the snapshot changed")
		return
	}
	s.scanning = true
	s.mu.Unlock()
	// Completion is independent of client disconnects; the explicit action owns one scan.
	snapshot, err := s.scan()
	var next http.Handler
	if err == nil {
		next = api.NewHandler(snapshot)
	}
	s.mu.Lock()
	s.scanning = false
	if err == nil {
		s.active = snapshot
		s.handler = next
	}
	s.mu.Unlock()
	if err != nil {
		fail(w, 500, "rescan_failed", fmt.Sprintf("rescan failed: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Nami-Snapshot", snapshot.ID)
	json.NewEncoder(w).Encode(map[string]string{"snapshot_id": snapshot.ID})
}

func localHost(host string) bool {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	return strings.EqualFold(name, "localhost") || name == "127.0.0.1" || name == "::1"
}
