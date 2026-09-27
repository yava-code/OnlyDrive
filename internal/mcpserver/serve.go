package mcpserver

import (
	"github.com/mark3labs/mcp-go/server"
)

// ServeStdio runs the MCP server over stdio until the client disconnects.
func ServeStdio() error {
	s := New()
	return server.ServeStdio(s)
}
