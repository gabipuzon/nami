package api

import (
	"fmt"
	"net/http"
	"net/url"
)

func parseParams(r *http.Request) (url.Values, error) {
	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("malformed query string")
	}
	return params, nil
}

func required(w http.ResponseWriter, params url.Values, key string) (string, bool) {
	values := params[key]
	if len(values) != 1 || values[0] == "" {
		writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("exactly one nonempty %s is required", key))
		return "", false
	}
	return values[0], true
}
