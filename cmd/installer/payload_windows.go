//go:build installer

package main

import _ "embed"

//go:embed payload/agent.exe
var embeddedAgent []byte

func init() {
	agent = embeddedAgent
}
