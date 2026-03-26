package api

import (
	"net/http"

	"github.com/swaggo/swag"
)

// scalarHTML is the Scalar API reference UI page.
// It loads the spec from /docs/swagger.json at runtime.
const scalarHTML = `<!doctype html>
<html lang="en">
  <head>
    <title>NORA API Reference</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <script
      id="api-reference"
      data-url="/docs/swagger.json"
    ></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`

// ScalarUI serves the Scalar API reference UI at GET /docs/.
func ScalarUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(scalarHTML))
}

// SwaggerJSON serves the generated OpenAPI 2.0 spec at GET /docs/swagger.json.
func SwaggerJSON(w http.ResponseWriter, r *http.Request) {
	doc, err := swag.ReadDoc()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "spec unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(doc))
}
