package api

import (
	"encoding/json"
	"net/http"
)

// RegisterDocsRoutes registers the public API docs endpoints on r.
// GET /docs   → Scalar interactive UI (CDN-loaded)
// GET /docs/openapi.json → raw OpenAPI 3.0 spec
func RegisterDocsRoutes(r interface {
	Get(pattern string, handlerFn http.HandlerFunc)
}) {
	r.Get("/docs", serveScalarUI)
	r.Get("/docs/openapi.json", serveOpenAPISpec)
}

// serveScalarUI returns a minimal HTML page that boots the Scalar API reference
// UI from CDN, pointed at our own /docs/openapi.json spec.
func serveScalarUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	//nolint:errcheck
	w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>NORA API Reference</title>
  <style>body { margin: 0; }</style>
</head>
<body>
  <script
    id="api-reference"
    data-url="/docs/openapi.json"
    data-configuration='{"theme":"purple"}'
  ></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`))
}

// serveOpenAPISpec returns the OpenAPI 3.0 spec for NORA as JSON.
func serveOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	spec := buildOpenAPISpec()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	//nolint:errcheck
	enc.Encode(spec)
}

// buildOpenAPISpec returns the full OpenAPI 3.0 document as a Go map.
// Keep this in sync with the actual routes registered in main.go.
func buildOpenAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "NORA – Nexus Operations Recon & Alerts",
			"description": "API for managing monitored apps, querying ingested events, and pushing webhook payloads.",
			"version":     "0.1.0",
		},
		"servers": []map[string]any{
			{"url": "/api/v1", "description": "Current server"},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"sessionCookie": map[string]any{
					"type": "apiKey",
					"in":   "cookie",
					"name": "nora_session",
				},
			},
			"schemas": map[string]any{
				"App": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "string", "format": "uuid"},
						"name":       map[string]any{"type": "string"},
						"profile_id": map[string]any{"type": "string"},
						"token":      map[string]any{"type": "string"},
						"rate_limit": map[string]any{"type": "integer"},
						"created_at": map[string]any{"type": "string", "format": "date-time"},
						"updated_at": map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"AppList": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"data":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/App"}},
						"total": map[string]any{"type": "integer"},
					},
				},
				"CreateAppRequest": map[string]any{
					"type":     "object",
					"required": []string{"name"},
					"properties": map[string]any{
						"name":       map[string]any{"type": "string"},
						"profile_id": map[string]any{"type": "string"},
						"config":     map[string]any{"type": "object"},
						"rate_limit": map[string]any{"type": "integer"},
					},
				},
				"Event": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":           map[string]any{"type": "string", "format": "uuid"},
						"app_id":       map[string]any{"type": "string", "format": "uuid"},
						"app_name":     map[string]any{"type": "string"},
						"received_at":  map[string]any{"type": "string", "format": "date-time"},
						"severity":     map[string]any{"type": "string", "enum": []string{"info", "warn", "error", "critical"}},
						"display_text": map[string]any{"type": "string"},
						"fields":       map[string]any{"type": "object"},
					},
				},
				"EventDetail": map[string]any{
					"allOf": []map[string]any{
						{"$ref": "#/components/schemas/Event"},
						{
							"type": "object",
							"properties": map[string]any{
								"raw_payload": map[string]any{"type": "object"},
							},
						},
					},
				},
				"EventList": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"data":   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Event"}},
						"total":  map[string]any{"type": "integer"},
						"limit":  map[string]any{"type": "integer"},
						"offset": map[string]any{"type": "integer"},
					},
				},
				"Error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error": map[string]any{"type": "string"},
					},
				},
				"TokenResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"token": map[string]any{"type": "string"},
					},
				},
			},
		},
		"security": []map[string]any{
			{"sessionCookie": []string{}},
		},
		"paths": map[string]any{
			// ── Apps ──────────────────────────────────────────────────────────────
			"/apps": map[string]any{
				"get": map[string]any{
					"summary":     "List all apps",
					"operationId": "listApps",
					"tags":        []string{"Apps"},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "OK",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/AppList"},
								},
							},
						},
						"401": errorResponse("Unauthorized"),
						"500": errorResponse("Internal server error"),
					},
				},
				"post": map[string]any{
					"summary":     "Create an app",
					"operationId": "createApp",
					"tags":        []string{"Apps"},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/CreateAppRequest"},
							},
						},
					},
					"responses": map[string]any{
						"201": map[string]any{
							"description": "Created",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/App"},
								},
							},
						},
						"400": errorResponse("Bad request"),
						"401": errorResponse("Unauthorized"),
						"500": errorResponse("Internal server error"),
					},
				},
			},
			"/apps/{id}": map[string]any{
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
				},
				"get": map[string]any{
					"summary":     "Get an app",
					"operationId": "getApp",
					"tags":        []string{"Apps"},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "OK",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/App"},
								},
							},
						},
						"401": errorResponse("Unauthorized"),
						"404": errorResponse("Not found"),
						"500": errorResponse("Internal server error"),
					},
				},
				"put": map[string]any{
					"summary":     "Update an app",
					"operationId": "updateApp",
					"tags":        []string{"Apps"},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/CreateAppRequest"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "OK",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/App"},
								},
							},
						},
						"400": errorResponse("Bad request"),
						"401": errorResponse("Unauthorized"),
						"404": errorResponse("Not found"),
						"500": errorResponse("Internal server error"),
					},
				},
				"delete": map[string]any{
					"summary":     "Delete an app",
					"operationId": "deleteApp",
					"tags":        []string{"Apps"},
					"responses": map[string]any{
						"204": map[string]any{"description": "Deleted"},
						"401": errorResponse("Unauthorized"),
						"404": errorResponse("Not found"),
						"500": errorResponse("Internal server error"),
					},
				},
			},
			"/apps/{id}/token/regenerate": map[string]any{
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
				},
				"post": map[string]any{
					"summary":     "Regenerate ingest token",
					"operationId": "regenerateToken",
					"tags":        []string{"Apps"},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "New token",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/TokenResponse"},
								},
							},
						},
						"401": errorResponse("Unauthorized"),
						"404": errorResponse("Not found"),
						"500": errorResponse("Internal server error"),
					},
				},
			},
			// ── Events ────────────────────────────────────────────────────────────
			"/events": map[string]any{
				"get": map[string]any{
					"summary":     "List events",
					"operationId": "listEvents",
					"tags":        []string{"Events"},
					"parameters": []map[string]any{
						{"name": "app_id", "in": "query", "schema": map[string]any{"type": "string", "format": "uuid"}},
						{"name": "severity", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"info", "warn", "error", "critical"}}},
						{"name": "since", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
						{"name": "until", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
						{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "default": 50}},
						{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "default": 0}},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "OK",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/EventList"},
								},
							},
						},
						"401": errorResponse("Unauthorized"),
						"500": errorResponse("Internal server error"),
					},
				},
			},
			"/events/{id}": map[string]any{
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
				},
				"get": map[string]any{
					"summary":     "Get an event",
					"operationId": "getEvent",
					"tags":        []string{"Events"},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "OK",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/EventDetail"},
								},
							},
						},
						"401": errorResponse("Unauthorized"),
						"404": errorResponse("Not found"),
						"500": errorResponse("Internal server error"),
					},
				},
			},
			"/apps/{id}/events": map[string]any{
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
				},
				"get": map[string]any{
					"summary":     "List events for an app",
					"operationId": "listAppEvents",
					"tags":        []string{"Events"},
					"parameters": []map[string]any{
						{"name": "severity", "in": "query", "schema": map[string]any{"type": "string"}},
						{"name": "since", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
						{"name": "until", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
						{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "default": 50}},
						{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "default": 0}},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "OK",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/EventList"},
								},
							},
						},
						"401": errorResponse("Unauthorized"),
						"404": errorResponse("Not found"),
						"500": errorResponse("Internal server error"),
					},
				},
			},
			// ── Ingest (public) ───────────────────────────────────────────────────
			"/ingest/{token}": map[string]any{
				"parameters": []map[string]any{
					{"name": "token", "in": "path", "required": true, "schema": map[string]any{"type": "string"}, "description": "App ingest token"},
				},
				"post": map[string]any{
					"summary":     "Ingest a webhook payload",
					"operationId": "ingestWebhook",
					"tags":        []string{"Ingest"},
					"security":    []map[string]any{},
					"servers": []map[string]any{
						{"url": "/api/v1", "description": "Current server (public — no auth required)"},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"type": "object"},
							},
						},
					},
					"responses": map[string]any{
						"202": map[string]any{"description": "Accepted"},
						"400": errorResponse("Bad request"),
						"401": errorResponse("Invalid token"),
						"429": errorResponse("Rate limit exceeded"),
						"500": errorResponse("Internal server error"),
					},
				},
			},
		},
	}
}

// errorResponse builds a standard error response object for OpenAPI.
func errorResponse(desc string) map[string]any {
	return map[string]any{
		"description": desc,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/Error"},
			},
		},
	}
}
