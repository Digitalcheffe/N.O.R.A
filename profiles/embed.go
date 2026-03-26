// Package profiles exposes the embedded YAML profile files.
package profiles

import "embed"

//go:embed *.yaml
var Files embed.FS
