package runtime

import "github.com/slighter12/godot-mcp-go/tools/types"

func GetAllTools() []types.Tool {
	return []types.Tool{
		&GetActiveGameSessionTool{},
		&RuntimeSyncNowTool{},
		&AwaitRuntimeSnapshotTool{},
		&RuntimeSceneTreeGetTool{},
		&RuntimeNodePropertiesGetTool{},
		&RuntimeInputTapTool{},
		&RuntimeInputPressTool{},
		&RuntimeInputReleaseTool{},
		&RuntimeLogGetTool{},
		&RuntimeLogClearTool{},
		&RuntimeScreenshotGetTool{},
		&BridgeEditorSyncTool{},
		&BridgeEditorPingTool{},
		&BridgeRuntimeRegisterTool{},
		&BridgeRuntimeSnapshotPushTool{},
		&BridgeRuntimeLogPushTool{},
		&BridgeCommandAckTool{},
	}
}
