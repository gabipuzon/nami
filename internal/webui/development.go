//go:build !webui

package webui

import "io/fs"

// CLI-only development builds do not silently serve a placeholder map.
func Files() fs.FS { return nil }
