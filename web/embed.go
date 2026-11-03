// Package web embeds the console so the binary is self-contained.
package web

import "embed"

// FS holds the static console files.
//
//go:embed index.html css js docs
var FS embed.FS
