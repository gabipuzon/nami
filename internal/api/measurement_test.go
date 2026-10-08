package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gabipuzon/nami/internal/storage"
)

// Opt-in terminal measurements use saved facts, with no source reads or browser automation.
func TestSavedSnapshotStartupMeasurements(t *testing.T) {
	paths := os.Getenv("NAMI_MEASURE_REPOSITORIES")
	if paths == "" {
		t.Skip("set NAMI_MEASURE_REPOSITORIES for local saved-snapshot measurements")
	}
	for _, root := range strings.Split(paths, ",") {
		store, err := storage.OpenReadOnly(root)
		if err != nil {
			t.Fatal(err)
		}
		scans, err := store.List()
		if err != nil {
			store.Close()
			t.Fatal(err)
		}
		if len(scans) == 0 {
			store.Close()
			t.Fatal("no saved scans", root)
		}
		snapshot, err := store.Load(scans[0].ID)
		store.Close()
		if err != nil {
			t.Fatal(err)
		}
		handler := NewHandler(snapshot)
		for _, route := range []string{"/api/v1/scan", "/api/v1/graph", "/api/v1/packages", "/api/v1/overview"} {
			elapsed := time.Duration(0)
			bytes := 0
			for i := 0; i < 5; i++ {
				w := httptest.NewRecorder()
				start := time.Now()
				handler.ServeHTTP(w, httptest.NewRequest("GET", route, nil))
				elapsed += time.Since(start)
				if w.Code == 404 {
					break
				}
				if w.Code != 200 {
					t.Fatal(route, w.Code, w.Body.String())
				}
				bytes = w.Body.Len()
				if route == "/api/v1/graph" && i == 0 {
					data, _ := json.Marshal(convertGraph(snapshot.Result.Graph))
					if err := os.WriteFile("/tmp/nami-measure-"+snapshot.ID+"-full.json", data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if route == "/api/v1/packages" && i == 0 {
					if err := os.WriteFile("/tmp/nami-measure-"+snapshot.ID+"-packages.json", w.Body.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if route == "/api/v1/overview" && i == 0 {
					if err := os.WriteFile("/tmp/nami-measure-"+snapshot.ID+"-overview.json", w.Body.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			t.Logf("root=%s snapshot=%s route=%s bytes=%d mean=%s", root, snapshot.ID, route, bytes, elapsed/5)
		}
	}
}
