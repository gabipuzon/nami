package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestAssetsAndAPIRouting(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("map")}, "_next/app.js": {Data: []byte("script")}}
	handler := Handler(files, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("api")) }))
	for path, want := range map[string]string{"/": "map", "/_next/app.js": "script", "/api/v1/scan": "api"} {
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, httptest.NewRequest("GET", path, nil))
		if out.Code != 200 || out.Body.String() != want {
			t.Fatalf("%s: %d %s", path, out.Code, out.Body.String())
		}
	}
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, httptest.NewRequest("GET", "/missing", nil))
	if out.Code != 404 {
		t.Fatalf("missing asset: %d", out.Code)
	}
}
