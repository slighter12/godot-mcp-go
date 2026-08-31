package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/slighter12/godot-mcp-go/internal/conformancefixture"
	"github.com/slighter12/godot-mcp-go/logger"
)

func main() {
	host := flag.String("host", "127.0.0.1", "loopback address to bind")
	port := flag.Int("port", 19090, "TCP port to bind")
	flag.Parse()

	if *host != "127.0.0.1" && *host != "localhost" && *host != "::1" {
		fmt.Fprintln(os.Stderr, "conformance fixture must bind to a loopback host")
		os.Exit(2)
	}
	if *port < 1 || *port > 65535 {
		fmt.Fprintln(os.Stderr, "conformance fixture port must be between 1 and 65535")
		os.Exit(2)
	}
	_ = logger.Init(logger.GetLevelFromString("info"), logger.FormatJSON)

	key, err := requestStateKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fixture, err := conformancefixture.New(key)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := fixture.NewHTTPServer(*host, *port).StartConformance(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func requestStateKey() ([]byte, error) {
	encoded := strings.TrimSpace(os.Getenv("MCP_REQUEST_STATE_KEY"))
	if encoded == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("MCP_REQUEST_STATE_KEY must be strict standard Base64: %w", err)
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("MCP_REQUEST_STATE_KEY must decode to at least 32 bytes")
	}
	return key, nil
}
