// Package api carries the OpenAPI description of the service.
//
// It lives outside internal/ because the contract is public: a client
// generating a stub from it is doing exactly what it is for. The document is
// embedded in the binary rather than read from disk, so the running build
// always serves the contract of the version it actually is, instead of one
// published separately and free to drift.
package api

import _ "embed"

// Spec is the OpenAPI 3.1 document, as YAML.
//
//go:embed openapi.yaml
var Spec []byte

// SpecContentType is the media type Spec should be served as.
const SpecContentType = "application/yaml; charset=utf-8"
