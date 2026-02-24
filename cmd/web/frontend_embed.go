//go:build embedfrontend

package web

import (
	"embed"
	"io/fs"
)

// embeddedFS holds the compiled frontend assets.
// Built by running `make build-web` or `yarn --cwd desktop build:web`.
//
//go:embed dist
var _embedded embed.FS

var embeddedFS fs.FS = _embedded
