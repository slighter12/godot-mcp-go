@tool
extends Node

signal connected
signal disconnected
signal error(error: String)
signal message_received(message: Dictionary)

var streamable_http_url: String = "http://localhost:9080/mcp"
var is_connecting: bool = false
var streamable_http_connection: HTTPRequest
var session_id: String = ""

func _ready():
    print("MCP Server: Initializing...")
    # Load settings.
    load_settings()
    
    # Initialize Streamable HTTP connection.
    streamable_http_connection = HTTPRequest.new()
    add_child(streamable_http_connection)
    streamable_http_connection.request_completed.connect(_on_streamable_http_request_completed)

func load_settings():
    print("MCP Server: Loading settings...")
    var config = ConfigFile.new()
    var err = config.load("res://addons/godot_mcp/config.cfg")
    if err == OK:
        streamable_http_url = config.get_value("mcp", "streamable_http_url", "http://localhost:9080/mcp")
        var configured_type = config.get_value("mcp", "connection_type", "streamable_http")
        if configured_type != "streamable_http":
            print("MCP Server: connection_type '%s' is unsupported in the plugin. Using streamable_http." % configured_type)
        print("MCP Server: Settings loaded - type: streamable_http, url: ", streamable_http_url)
    else:
        print("MCP Server: Failed to load settings, using defaults")

func connect_to_server():
    print("MCP Server: Attempting to connect...")
    connect_streamable_http(streamable_http_url)

func connect_streamable_http(url: String):
    if is_connecting:
        print("MCP Server: Already attempting to connect...")
        return
        
    is_connecting = true
    print("MCP Server: Connecting to Streamable HTTP at ", url)
    
    # Send initialize request.
    var init_message = {
        "jsonrpc": "2.0",
        "id": "init",
        "method": "initialize",
        "params": {
            "protocolVersion": "2025-03-26",
            "capabilities": {},
            "clientInfo": {
                "name": "godot-mcp",
                "version": "0.1.0"
            }
        }
    }
    
    var headers = [
        "Content-Type: application/json",
        "Accept: application/json, text/event-stream"
    ]
    
    var json_message = JSON.stringify(init_message)
    streamable_http_connection.request(url, headers, HTTPClient.METHOD_POST, json_message)

func _on_streamable_http_request_completed(result: int, response_code: int, headers: PackedStringArray, body: PackedByteArray):
    if result != HTTPRequest.RESULT_SUCCESS:
        is_connecting = false
        print("MCP Server: Streamable HTTP request failed with result: ", result)
        emit_signal("error", "Streamable HTTP request failed: " + str(result))
        return

    print("MCP Server: Streamable HTTP response received - code: ", response_code)

    # This sample currently expects request/response calls to return HTTP 200 with JSON.
    # MCP notifications may legally return HTTP 202 (Accepted) without a response body,
    # but that notification-only flow is intentionally not handled in this reference.
    if response_code != 200:
        is_connecting = false
        print("MCP Server: Streamable HTTP request failed with status: ", response_code)
        emit_signal("error", "Streamable HTTP request failed with status: " + str(response_code))
        return

    var response_text = body.get_string_from_utf8()
    print("MCP Server: Response body: ", response_text)

    var json = JSON.new()
    var err = json.parse(response_text)
    if err != OK:
        is_connecting = false
        print("MCP Server: Failed to parse response JSON")
        emit_signal("error", "Failed to parse response JSON")
        return

    var response = json.get_data()
    if is_connecting:
        if not (response.has("result") and response["result"].has("type") and response["result"]["type"] == "init"):
            is_connecting = false
            print("MCP Server: Unexpected response format")
            emit_signal("error", "Unexpected response format")
            return

        print("MCP Server: Initialization successful")
        for header in headers:
            if header.begins_with("Mcp-Session-Id: "):
                session_id = header.split(": ")[1]
                print("MCP Server: Session ID received: ", session_id)
                break

        is_connecting = false
        emit_signal("connected")
        establish_sse_stream()
        return

    if response is Dictionary:
        emit_signal("message_received", response)
    else:
        emit_signal("error", "Unexpected response payload")

func establish_sse_stream():
    print("MCP Server: Establishing SSE stream for server-to-client communication...")
    
    var headers = [
        "Accept: text/event-stream",
        "Mcp-Session-Id: " + session_id
    ]
    
    streamable_http_connection.request(streamable_http_url, headers, HTTPClient.METHOD_GET)

func send_message(message: Dictionary):
    print("MCP Server: Sending message: ", message)
    var json_message = JSON.stringify(message)
    if streamable_http_connection and session_id != "":
        var headers = [
            "Content-Type: application/json",
            "Accept: application/json, text/event-stream",
            "Mcp-Session-Id: " + session_id
        ]
        streamable_http_connection.request(streamable_http_url, headers, HTTPClient.METHOD_POST, json_message)
        print("MCP Server: Message sent via Streamable HTTP")
    else:
        print("MCP Server: Streamable HTTP not initialized or no session ID")
