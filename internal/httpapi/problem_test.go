package httpapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every problem carries a type URL pointing at a section of docs/errors.md.
// Nothing but a test holds the two together, and for a while nothing did: the
// document was missing entirely while every error response linked to it, so
// each one shipped a dead link. These tests make that impossible to repeat.

// problemSlugs reads the slugs the constructors pass to newProblem.
//
// The source is read as text rather than walked as a syntax tree. What is being
// matched is a literal argument in a fixed position, and a regexp says that as
// clearly as an AST walk would while staying short enough to read.
func problemSlugs(t *testing.T) []string {
	t.Helper()

	data, err := os.ReadFile("problem.go")
	if err != nil {
		t.Fatalf("could not read problem.go: %v", err)
	}

	// newProblem(http.StatusBadRequest, "bad-request", "Malformed request", ...)
	call := regexp.MustCompile(`newProblem\([^,]+,\s*"([a-z0-9-]+)"`)

	var out []string
	for _, m := range call.FindAllStringSubmatch(string(data), -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatal("found no problem slugs in problem.go; the shape being matched must have changed")
	}
	return out
}

// docAnchors returns the anchors GitHub will generate for the headings in
// docs/errors.md: lowercased, punctuation dropped, spaces hyphenated.
func docAnchors(t *testing.T) map[string]bool {
	t.Helper()

	path := filepath.Join("..", "..", "docs", "errors.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read %s, which every error response links to: %v", path, err)
	}

	heading := regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*$`)
	drop := regexp.MustCompile(`[^a-z0-9 -]`)

	out := map[string]bool{}
	for _, m := range heading.FindAllStringSubmatch(string(data), -1) {
		slug := drop.ReplaceAllString(strings.ToLower(m[1]), "")
		out[strings.ReplaceAll(slug, " ", "-")] = true
	}
	return out
}

func TestEveryProblemTypeHasSomethingToLinkTo(t *testing.T) {
	anchors := docAnchors(t)
	for _, slug := range problemSlugs(t) {
		if !anchors[slug] {
			t.Errorf("problems of type %q link to docs/errors.md#%s, which has no heading; "+
				"a caller following the link would land nowhere", slug, slug)
		}
	}
}

func TestProblemTypeURLPointsAtThatDocument(t *testing.T) {
	// The slug check above would still pass if the base URL named a file that
	// does not exist, so the path itself is asserted.
	const want = "/docs/errors.md#"
	if !strings.HasSuffix(docsBase, want) {
		t.Errorf("docsBase = %q, want it to end in %q so the anchors resolve against "+
			"the document this repository actually carries", docsBase, want)
	}
}

// TestProblemConstructorsAreDocumented checks the other direction for the
// constructors themselves, so one added without a section is caught by the type
// URL it produces rather than by a reader following a dead link.
func TestProblemConstructorsAreDocumented(t *testing.T) {
	anchors := docAnchors(t)

	problems := []*Problem{
		BadRequest("x"),
		Unauthorized("x"),
		NotFound("x"),
		Validation("x"),
		PayloadTooLarge("x"),
		Internal(),
	}
	for _, p := range problems {
		slug, ok := strings.CutPrefix(p.Type, docsBase)
		if !ok {
			t.Errorf("%s: type %q does not start with the documentation base", p.Title, p.Type)
			continue
		}
		if !anchors[slug] {
			t.Errorf("%s (%d) links to #%s, which docs/errors.md does not document",
				p.Title, p.Status, slug)
		}
	}
}
