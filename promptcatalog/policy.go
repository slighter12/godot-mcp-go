package promptcatalog

// PolicyCheck describes one machine-readable policy rule.
type PolicyCheck struct {
	ID          string   `json:"id"`
	Level       string   `json:"level"`
	AppliesTo   []string `json:"appliesTo"`
	Description string   `json:"description"`
	StopAndAsk  bool     `json:"stopAndAsk"`
}

// GodotPolicyChecks returns the baseline policy-godot check catalog.
func GodotPolicyChecks() []PolicyCheck {
	return []PolicyCheck{
		{ID: "SCENE-ORG", Level: "error", AppliesTo: []string{"*.tscn", "**/*.tscn", "*.gd", "**/*.gd", "*.rs", "**/*.rs"}, Description: "Scenes and their primary scripts must be colocated in PascalCase folders.", StopAndAsk: true},
		{ID: "SCENE-HIERARCHY", Level: "error", AppliesTo: []string{"*.tscn", "**/*.tscn"}, Description: "Use correct root node type and keep hierarchy depth at or below 5.", StopAndAsk: true},
		{ID: "SCENE-NAMING", Level: "warn", AppliesTo: []string{"*.tscn", "**/*.tscn"}, Description: "Node names should be PascalCase and purpose-driven."},
		{ID: "SCENE-AUTOLOAD", Level: "warn", AppliesTo: []string{"project.godot"}, Description: "Keep autoload count low and place scripts under res://autoload/.", StopAndAsk: true},
		{ID: "LIFECYCLE-READY", Level: "error", AppliesTo: []string{"*.gd", "**/*.gd", "*.rs", "**/*.rs"}, Description: "Access child nodes only in _ready() or later.", StopAndAsk: true},
		{ID: "LIFECYCLE-PHYSICS", Level: "error", AppliesTo: []string{"*.gd", "**/*.gd"}, Description: "Do not mutate physics bodies in _process(); use _physics_process().", StopAndAsk: true},
		{ID: "LIFECYCLE-DELTA", Level: "warn", AppliesTo: []string{"*.gd", "**/*.gd", "*.rs", "**/*.rs"}, Description: "Time-based updates should use delta."},
		{ID: "SIGNAL-NAMING", Level: "error", AppliesTo: []string{"*.gd", "**/*.gd", "*.rs", "**/*.rs"}, Description: "Signals must use snake_case, past-tense naming, and typed parameters.", StopAndAsk: true},
		{ID: "SIGNAL-CONNECTION", Level: "warn", AppliesTo: []string{"*.gd", "**/*.gd", "*.rs", "**/*.rs"}, Description: "Connect in _ready() and clean up connections in _exit_tree().", StopAndAsk: true},
		{ID: "RESOURCE-LOAD", Level: "error", AppliesTo: []string{"*.gd", "**/*.gd", "*.rs", "**/*.rs", "*.tscn", "**/*.tscn"}, Description: "Avoid load() inside per-frame loops and prefer preload() for known assets.", StopAndAsk: true},
		{ID: "RESOURCE-PATHS", Level: "warn", AppliesTo: []string{"*.gd", "**/*.gd", "*.rs", "**/*.rs", "*.tscn", "**/*.tscn"}, Description: "Use res:// paths and avoid absolute paths."},
		{ID: "GD-TYPING", Level: "error", AppliesTo: []string{"*.gd", "**/*.gd"}, Description: "Require type hints for declarations, parameters, and returns.", StopAndAsk: true},
		{ID: "GD-NAMING", Level: "warn", AppliesTo: []string{"*.gd", "**/*.gd"}, Description: "Use snake_case for vars/functions and UPPER_SNAKE_CASE for constants."},
		{ID: "RUST-GDEXT", Level: "error", AppliesTo: []string{"Cargo.toml"}, Description: "Use godot-rust/gdext aligned with the target Godot 4 runtime.", StopAndAsk: true},
		{ID: "RUST-OWNERSHIP", Level: "warn", AppliesTo: []string{"*.rs", "**/*.rs"}, Description: "Prefer Gd<T> and temporary bind()/bind_mut() borrows over raw references.", StopAndAsk: true},
	}
}
