//go:build installer

package main

import _ "embed"

//go:embed payload/agent.exe
var embeddedAgent []byte

//go:embed payload/tray.exe
var embeddedTray []byte

func init() {
	agent = embeddedAgent
	tray = embeddedTray
}
