package main

import (
	_ "embed"
)

//go:embed icon.ico
var embeddedIcon []byte

// generateTrayIcon returns the embedded icon
func generateTrayIcon() []byte {
	return embeddedIcon
}
