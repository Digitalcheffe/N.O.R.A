package frontend

import "embed"

// Dist holds the compiled React app from frontend/dist, embedded at build time.
// Locally the dist/ directory contains only a .gitkeep placeholder.
// In Docker, the Dockerfile copies the Vite build output here before go build.
//
//go:embed all:dist
var Dist embed.FS
