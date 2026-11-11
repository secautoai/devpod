//go:build !embedfrontend

package web

import "io/fs"

// embeddedFS is nil when the binary is built without the `embedfrontend` build tag.
// In this case the web handler returns a helpful error message pointing the user
// to the build instructions.
var embeddedFS fs.FS = nil
