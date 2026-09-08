package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/api"
	"github.com/casbek/api-swiss-ephemeris/internal/config"
)

// A published contract that quietly falls behind the code is worse than none:
// a client generating a stub from it would build against endpoints that do not
// exist, or miss ones that do. These tests hold the document and the route
// table to each other, so neither can move without the other.

// specPaths extracts the paths and methods the document declares.
//
// The document is read as text rather than parsed, since the repository carries
// no YAML parser and pulling one in for a handful of lines would be a
// dependency the service does not otherwise need. The shape being matched is
// fixed by the document's own layout, which lint keeps honest.
func specPaths(t *testing.T) map[string]map[string]bool {
	t.Helper()

	var (
		pathLine   = regexp.MustCompile(`^  (/\S*):\s*$`)
		methodLine = regexp.MustCompile(`^    (get|post|put|patch|delete|head|options):\s*$`)
	)

	out := map[string]map[string]bool{}
	inPaths := false
	current := ""

	for _, line := range strings.Split(string(api.Spec), "\n") {
		line = strings.TrimRight(line, "\r")

		// The paths block runs from `paths:` to the next top-level key.
		if line == "paths:" {
			inPaths = true
			continue
		}
		if inPaths && len(line) > 0 && line[0] != ' ' {
			break
		}
		if !inPaths {
			continue
		}

		if m := pathLine.FindStringSubmatch(line); m != nil {
			current = m[1]
			out[current] = map[string]bool{}
			continue
		}
		if m := methodLine.FindStringSubmatch(line); m != nil && current != "" {
			out[current][strings.ToUpper(m[1])] = true
		}
	}

	if len(out) == 0 {
		t.Fatal("no paths were found in the OpenAPI document; the layout it is read with has changed")
	}
	return out
}

// TestEveryRouteIsDocumented checks that nothing the service answers is missing
// from the contract.
func TestEveryRouteIsDocumented(t *testing.T) {
	documented := specPaths(t)

	for _, rt := range routeTable() {
		methods, ok := documented[rt.Path]
		if !ok {
			t.Errorf("%s %s is served but does not appear in the OpenAPI document",
				rt.Method, rt.Path)
			continue
		}
		if !methods[rt.Method] {
			t.Errorf("%s is documented, but not for %s", rt.Path, rt.Method)
		}
	}
}

// TestEveryDocumentedPathIsServed checks the other direction: the contract must
// not promise an endpoint that does not exist.
func TestEveryDocumentedPathIsServed(t *testing.T) {
	served := map[string]map[string]bool{}
	for _, rt := range routeTable() {
		if served[rt.Path] == nil {
			served[rt.Path] = map[string]bool{}
		}
		served[rt.Path][rt.Method] = true
	}

	for path, methods := range specPaths(t) {
		for method := range methods {
			if !served[path][method] {
				t.Errorf("the OpenAPI document promises %s %s, which the service does not answer",
					method, path)
			}
		}
	}
}

// TestOpenAPIEndpoint checks that the document is served, and served as the
// running build's own copy.
func TestOpenAPIEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	rec := do(t, s, http.MethodGet, openAPIPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}

	body := rec.Body.String()
	if !strings.HasPrefix(body, "openapi: 3.1.0") {
		t.Errorf("the body does not begin with the OpenAPI version: %.40q", body)
	}
	if body != string(api.Spec) {
		t.Error("the served document differs from the embedded one")
	}

	// It only changes when a new build is deployed, so it should revalidate
	// rather than be fetched again.
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag was set")
	}
	second := do(t, s, http.MethodGet, openAPIPath, map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified {
		t.Errorf("status = %d on revalidation, want 304", second.Code)
	}
}

// TestOpenAPIIsReachableWithoutKey checks the exemption. A client deciding
// whether to integrate has to be able to read the contract before it has a key.
func TestOpenAPIIsReachableWithoutKey(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) { c.APIKeys = []string{"secret-key"} })

	if rec := do(t, s, http.MethodGet, openAPIPath, nil); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 without a key", rec.Code)
	}
}

// TestIndexPointsAtTheDocument checks that a client landing on the index is
// told where the contract is.
func TestIndexPointsAtTheDocument(t *testing.T) {
	s := newTestServer(t, nil)

	rec := do(t, s, http.MethodGet, "/v1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		OpenAPI   string     `json:"openapi"`
		Endpoints []endpoint `json:"endpoints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if body.OpenAPI != openAPIPath {
		t.Errorf("openapi = %q, want %q", body.OpenAPI, openAPIPath)
	}
	// The index is built from the same table the mux is, so the two cannot
	// disagree about what exists.
	if len(body.Endpoints) != len(routeTable()) {
		t.Errorf("the index lists %d endpoints but the service answers %d",
			len(body.Endpoints), len(routeTable()))
	}
}
