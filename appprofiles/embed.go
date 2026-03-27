// Package appprofiles exposes the embedded YAML app template files.
package appprofiles

import "embed"

//go:embed *.yaml
var Files embed.FS
