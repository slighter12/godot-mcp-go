# Godot MCP Go Server

A Go-based implementation of the Model Context Protocol (MCP) server for Godot, providing seamless integration between Godot and AI assistants like Claude and Cursor.

This project is based on the design of [ee0pdt/Godot-MCP](https://github.com/ee0pdt/Godot-MCP) and implements the MCP protocol in Go, with references to [metoro-io/mcp-golang](https://github.com/metoro-io/mcp-golang) and [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go/blob/main/mcp/utils.go) for protocol implementation details.

## Features

- **Dual Transport Support**: Both stdio and Streamable HTTP transports
- **Modern Architecture**: Clean separation of concerns with echo framework
- **Comprehensive Tool System**: Organized tool categories with interface-based design
- **Session Management**: Robust session handling for Streamable HTTP
- **Robust Error Handling**: Comprehensive logging and error management
- **Easy Configuration**: JSON-based configuration with environment variable support

## Architecture

### Transport Layer

- **stdio**: Direct process communication via standard input/output
- **Streamable HTTP**: HTTP-based communication (POST-only in this implementation)
  - POST requests for client-to-server request/notification/response delivery
  - Session management with unique session IDs

### Tool System

Tools are organized into categories and implement the `types.Tool` interface:

- **Node Tools**: Scene tree manipulation, node properties, creation/deletion
- **Script Tools**: Script management, reading, writing, analysis
- **Scene Tools**: Scene management, listing, creation, application
- **Project Tools**: Project settings, resource management, editor state
- **Utility Tools**: General utilities and offerings

### Directory Structure

```text
transport/
├── http/           # HTTP transport implementation
│   ├── server.go   # HTTP server with echo framework
│   ├── router.go   # HTTP routes and handlers
│   ├── streamable.go # Streamable HTTP transport
│   └── session.go  # Session management
├── stdio/          # stdio transport implementation
│   ├── server.go   # stdio server
│   └── test.go     # stdio tests
└── logs/           # Log files
```

## Prerequisites

- Go 1.26 or later
- Godot 4.x
- Basic understanding of the Model Context Protocol

## Installation

1. Clone the repository:

   ```bash
   git clone https://github.com/slighter12/godot-mcp-go.git
   cd godot-mcp-go
   ```

2. Install dependencies:

   ```bash
   go mod tidy
   ```

3. Build the server:

   ```bash
   go build
   ```

4. Link the MCP plugin to your Godot project:

   ```bash
   # Create the addons directory if it doesn't exist
   mkdir -p /path/to/your/godot/project/addons
   
   # Create a symbolic link to the MCP plugin
   ln -s /path/to/godot-mcp-go/godot-plugin /path/to/your/godot/project/addons/godot_mcp
   ```

## Usage

### Streamable HTTP Mode (Default)

1. Start the server:

   ```bash
   ./godot-mcp-go
   ```

2. The server will be available at:
   - MCP endpoint: `http://localhost:9080/mcp`
   - HTTP info endpoint: `http://localhost:9080`

### Stdio Mode

1. Start the server with stdio mode:

   ```bash
   MCP_USE_STDIO=true ./godot-mcp-go
   ```

### Godot Plugin Transport Note

- The Godot editor plugin currently supports `streamable_http` only.
- The server still supports both `stdio` and `streamable_http` transports for non-plugin clients.

## Configuration

The server can be configured through `config/mcp_config.json`:

```json
{
  "name": "godot-mcp-go",
  "version": "0.1.0",
  "description": "Go-based Model Context Protocol server for Godot",
  "server": {
    "host": "localhost",
    "port": 9080,
    "debug": false
  },
  "transports": [
    {
      "type": "stdio",
      "enabled": true
    },
    {
      "type": "streamable_http",
      "enabled": true,
      "url": "http://localhost:9080/mcp",
      "headers": {
        "Accept": "application/json, text/event-stream",
        "Content-Type": "application/json",
        "MCP-Protocol-Version": "2025-11-25"
      }
    }
  ],
  "logging": {
    "level": "debug",
    "format": "json",
    "path": "logs/mcp.log"
  },
  "prompt_catalog": {
    "enabled": true,
    "paths": [],
    "allowed_roots": [],
    "auto_reload": {
      "enabled": false,
      "interval_seconds": 5
    },
    "rendering": {
      "mode": "legacy",
      "reject_unknown_arguments": false
    }
  }
}
```

On startup, the server resolves the config path in this order:
1. `MCP_CONFIG_PATH`
2. `config/mcp_config.json` (project local, if present)
3. `~/.godot-mcp/config/mcp_config.json`

If the resolved file does not exist, the server creates a default config file at that path.

### Environment Variables

- `MCP_USE_STDIO`: Set to "true" to use stdio transport on the Go server
- `MCP_DEBUG`: Set to "true" to enable debug mode
- `MCP_CONFIG_PATH`: Override config file path
- `MCP_PORT`: Override server port
- `MCP_HOST`: Override server host
- `MCP_LOG_LEVEL`: Override log level
- `MCP_LOG_PATH`: Override log path
- `MCP_PROMPT_CATALOG_ENABLED`: Enable/disable prompt catalog endpoints
- `MCP_PROMPT_CATALOG_PATHS`: Comma-separated paths scanned for `SKILL.md`
- `MCP_PROMPT_CATALOG_ALLOWED_ROOTS`: Comma-separated allow-list roots for discovered `SKILL.md`
- `MCP_PROMPT_CATALOG_AUTO_RELOAD_ENABLED`: Enable polling-based prompt catalog reload
- `MCP_PROMPT_CATALOG_AUTO_RELOAD_INTERVAL_SECONDS`: Poll interval in seconds (`2..300`)
- `MCP_PROMPT_CATALOG_RENDERING_MODE`: Prompt rendering mode (`legacy` or `strict`)
- `MCP_PROMPT_CATALOG_REJECT_UNKNOWN_ARGUMENTS`: Reject unknown prompt arguments in strict mode

### Prompt Catalog Runtime Notes

- `prompt_catalog.enabled=false` disables prompt catalog endpoints; `prompts/list` and `prompts/get` return semantic `kind=not_supported`.
- `reload-prompt-catalog` is the manual runtime reload entrypoint exposed through `tools/call`.
- `notifications/prompts/list_changed` is emitted only when visible prompt metadata changes.
- `allowed_roots` constrains discovered `SKILL.md` files; if empty, the runtime falls back to `paths`.
- `rendering.mode=legacy` preserves existing behavior.
- `rendering.mode=strict` validates required placeholders before rendering, and optionally rejects unknown arguments when `reject_unknown_arguments=true`.
- `auto_reload.enabled=true` enables polling-based source fingerprint checks (`SKILL.md` path + size + mtime + content SHA-256) and triggers the same reload pipeline as `reload-prompt-catalog`.

## Available Tools

### Scene Tools

- `list-project-scenes`: Lists all `.tscn` files in the project
- `read-scene`: Reads a specific scene file
- `create-scene`: Creates a new scene
- `save-scene`: Saves the current scene
- `apply-scene`: Applies a scene to the current project

### Node Tools

- `get-scene-tree`: Returns the scene tree structure
- `get-node-properties`: Gets properties of a specific node
- `create-node`: Creates a new node
- `delete-node`: Deletes a node
- `modify-node`: Updates node properties

### Script Tools

- `list-project-scripts`: Lists all scripts in the project
- `read-script`: Reads a specific script
- `modify-script`: Modifies a script
- `create-script`: Creates a new script
- `analyze-script`: Analyzes a script

### Project Tools

- `get-project-settings`: Gets project settings
- `list-project-resources`: Lists all resources in the project
- `get-editor-state`: Gets the current editor state
- `run-project`: Runs the project
- `stop-project`: Stops the running project

## Cursor Configuration

To enable MCP integration with Cursor IDE, create or modify:

`~/.cursor/mcp.json`:

```json
{
    "mcpServers": {
        "godot-mcp": {
            "type": "streamable_http",
            "url": "http://localhost:9080/mcp"
        }
    }
}
```

## Development

### Building and Testing

```bash
# Build the project
go build

# Run tests
go test ./...

# Run with debug logging
MCP_DEBUG=true go run main.go
```

### Project Structure

```text
godot-mcp-go/
├── config/          # Configuration management
├── docs/            # Planning and architecture documents
├── logger/          # Logging system
├── mcp/             # MCP protocol implementation
├── promptcatalog/   # Prompt catalog discovery and policy metadata
├── tools/           # Tool system implementation
│   ├── node/        # Node-related tools
│   ├── script/      # Script-related tools
│   ├── scene/       # Scene-related tools
│   ├── project/     # Project-related tools
│   ├── utility/     # Utility tools
│   └── types/       # Tool interfaces and types
├── transport/       # Transport layer implementation
│   ├── http/        # HTTP transport
│   └── stdio/       # stdio transport
├── godot-plugin/    # Godot plugin
└── main.go          # Application entry point
```

### Planning Docs

- `docs/DEVELOPMENT.md`: Repository-level roadmap and milestone ownership
- `docs/PROMPT_CATALOG_COMPLETENESS_PLAN_V1.md`: Prompt catalog runtime contract and delivery phases

### Referenced Skill Sources

- [SkillsMP: godot-gdscript-patterns](https://skillsmp.com/skills/wshobson-agents-plugins-game-development-skills-godot-gdscript-patterns-skill-md)
- [wshobson/agents: godot-gdscript-patterns SKILL.md](https://raw.githubusercontent.com/wshobson/agents/main/plugins/game-development/skills/godot-gdscript-patterns/SKILL.md)
- [jwynia/agent-skills: godot-best-practices SKILL.md](https://raw.githubusercontent.com/jwynia/agent-skills/main/skills/tech/game-development/godot/godot-best-practices/SKILL.md)
- [bfollington/terma: godot SKILL.md](https://raw.githubusercontent.com/bfollington/terma/main/plugins/tsal/skills/godot/SKILL.md)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## References

### Core Technologies

- [Model Context Protocol](https://modelcontextprotocol.io/) - Official MCP documentation
- [MCP Specification](https://modelcontextprotocol.io/specification/) - Protocol specification
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification) - JSON-RPC protocol

### Frameworks and Libraries

- [Echo Framework](https://echo.labstack.com/) - HTTP web framework for Go
- [Godot Engine](https://godotengine.org/) - Game engine and editor

### Related Projects

- [ee0pdt/Godot-MCP](https://github.com/ee0pdt/Godot-MCP) - Original Godot MCP implementation
- [metoro-io/mcp-golang](https://github.com/metoro-io/mcp-golang) - Go MCP reference implementation
- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) - Another Go MCP implementation
