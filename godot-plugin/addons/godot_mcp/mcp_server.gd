@tool
extends Node

signal connected
signal disconnected
signal error(error: String)
signal message_received(message: Dictionary)

var stdio_pid: int = -1
var connection_type: String = "streamable_http"  # 或 "stdio" 或 "streamable_http"
var streamable_http_url: String = "http://localhost:9080/mcp"  # Streamable HTTP 端點
var server_command: String = "./godot-mcp-go"
var is_connecting: bool = false
var streamable_http_connection: HTTPRequest
var session_id: String = ""

func _ready():
    print("MCP Server: Initializing...")
    # 加載設置
    load_settings()
    
    # 初始化 Streamable HTTP 連接
    streamable_http_connection = HTTPRequest.new()
    add_child(streamable_http_connection)
    streamable_http_connection.request_completed.connect(_on_streamable_http_request_completed)

func load_settings():
    print("MCP Server: Loading settings...")
    var config = ConfigFile.new()
    var err = config.load("res://addons/godot_mcp/config.cfg")
    if err == OK:
        connection_type = config.get_value("mcp", "connection_type", "streamable_http")
        streamable_http_url = config.get_value("mcp", "streamable_http_url", "http://localhost:9080/mcp")
        server_command = config.get_value("mcp", "server_command", "./godot-mcp-go")
        print("MCP Server: Settings loaded - type: ", connection_type, ", url: ", streamable_http_url)
    else:
        print("MCP Server: Failed to load settings, using defaults")

func connect_to_server():
    print("MCP Server: Attempting to connect...")
    match connection_type:
        "stdio":
            connect_stdio()
        "streamable_http":
            connect_streamable_http(streamable_http_url)
        _:
            emit_signal("error", "Invalid connection type: " + connection_type)

func connect_streamable_http(url: String):
    if is_connecting:
        print("MCP Server: Already attempting to connect...")
        return
        
    is_connecting = true
    print("MCP Server: Connecting to Streamable HTTP at ", url)
    
    # 發送初始化請求
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
    
    # 檢查是否為初始化響應
    if response_code == 200:
        var response_text = body.get_string_from_utf8()
        print("MCP Server: Response body: ", response_text)
        
        var json = JSON.new()
        var err = json.parse(response_text)
        if err == OK:
            var response = json.get_data()
            
            # 檢查是否為初始化響應
            if response.has("result") and response["result"].has("type") and response["result"]["type"] == "init":
                print("MCP Server: Initialization successful")
                
                # 從響應頭中獲取會話 ID
    for header in headers:
                    if header.begins_with("Mcp-Session-Id: "):
                        session_id = header.split(": ")[1]
                        print("MCP Server: Session ID received: ", session_id)
            break
            
                is_connecting = false
                emit_signal("connected")
                
                # 建立 SSE 流以接收服務器消息
                establish_sse_stream()
            else:
                print("MCP Server: Unexpected response format")
                emit_signal("error", "Unexpected response format")
        else:
            print("MCP Server: Failed to parse response JSON")
            emit_signal("error", "Failed to parse response JSON")
                else:
        is_connecting = false
        print("MCP Server: Streamable HTTP request failed with status: ", response_code)
        emit_signal("error", "Streamable HTTP request failed with status: " + str(response_code))

func establish_sse_stream():
    print("MCP Server: Establishing SSE stream for server-to-client communication...")
    
    var headers = [
        "Accept: text/event-stream",
        "Mcp-Session-Id: " + session_id
    ]
    
    streamable_http_connection.request(streamable_http_url, headers, HTTPClient.METHOD_GET)

func connect_stdio():
    print("MCP Server: Starting stdio process: ", server_command)
    var args = []
    var env = ["MCP_USE_STDIO=true"]
    
    stdio_pid = OS.create_process(server_command, args, env)
    if stdio_pid == -1:
        print("MCP Server: Failed to start stdio process")
        emit_signal("error", "Failed to start stdio process")
        return
    
    print("MCP Server: Stdio process started with PID: ", stdio_pid)
    emit_signal("connected")
    # 發送初始化消息
    send_init_message()

func send_init_message():
    print("MCP Server: Sending init message...")
    var init_message = {
        "type": "init",
        "payload": {}
    }
    send_message(init_message)

func send_message(message: Dictionary):
    print("MCP Server: Sending message: ", message)
    var json_message = JSON.stringify(message)
    match connection_type:
        "stdio":
            if stdio_pid != -1 and OS.is_process_running(stdio_pid):
                # 使用系統命令發送消息
                var output = []
                var exit_code = OS.execute("echo", [json_message], output, true, stdio_pid)
                if exit_code != 0:
                    print("MCP Server: Failed to send message to stdio process")
                    emit_signal("error", "Failed to send message to stdio process")
                else:
                    print("MCP Server: Message sent via stdio")
            else:
                print("MCP Server: Stdio process not running")
        "streamable_http":
            # 使用 HTTP POST 發送消息
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
        _:
            print("MCP Server: Invalid connection type")
            emit_signal("error", "Invalid connection type: " + connection_type)

func _process(_delta):
    if stdio_pid != -1 and not OS.is_process_running(stdio_pid):
        print("MCP Server: Process terminated unexpectedly")
        emit_signal("error", "Process terminated unexpectedly")
        stdio_pid = -1 