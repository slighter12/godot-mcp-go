package promptcatalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Prompt represents one prompt exposed through MCP prompt endpoints.
type Prompt struct {
	Name        string
	Title       string
	Description string
	Arguments   []PromptArgument
	Template    string
	SourcePath  string
}

// PromptArgument describes one named template argument.
type PromptArgument struct {
	Name     string
	Required bool
}

var promptArgumentPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

// Registry stores prompts discovered from skill files.
type Registry struct {
	enabled bool

	mu         sync.RWMutex
	prompts    map[string]Prompt
	loadErrors []string
}

// NewRegistry creates a registry instance.
func NewRegistry(enabled bool) *Registry {
	return &Registry{
		enabled: enabled,
		prompts: make(map[string]Prompt),
	}
}

// Enabled reports whether skill-based prompt features are enabled.
func (r *Registry) Enabled() bool {
	return r != nil && r.enabled
}

// RegisterPrompt inserts or replaces one prompt definition.
func (r *Registry) RegisterPrompt(prompt Prompt) {
	if r == nil {
		return
	}

	name := strings.TrimSpace(prompt.Name)
	if name == "" {
		return
	}

	prompt.Name = name
	prompt = normalizePrompt(prompt)
	key := promptKey(prompt.Name)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.prompts[key] = prompt
}

// PromptCount returns the number of registered prompts.
func (r *Registry) PromptCount() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.prompts)
}

// ListPrompts returns prompt definitions sorted by name.
func (r *Registry) ListPrompts() []Prompt {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Prompt, 0, len(r.prompts))
	for _, prompt := range r.prompts {
		out = append(out, prompt)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// GetPrompt returns one prompt by name.
func (r *Registry) GetPrompt(name string) (Prompt, bool) {
	if r == nil {
		return Prompt{}, false
	}

	key := promptKey(name)
	if key == "" {
		return Prompt{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	prompt, ok := r.prompts[key]
	return prompt, ok
}

// LoadErrors returns non-fatal errors seen during skill discovery.
func (r *Registry) LoadErrors() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.loadErrors))
	copy(out, r.loadErrors)
	return out
}

// LoadFromPaths discovers SKILL.md files and registers prompt metadata.
func (r *Registry) LoadFromPaths(paths []string) error {
	if !r.Enabled() {
		return nil
	}

	files := make([]string, 0)
	seen := make(map[string]struct{})
	loadErrors := make([]string, 0)
	for _, rawPath := range paths {
		discovered, discoverErr := discoverSkillFiles(rawPath)
		if discoverErr != nil {
			loadErrors = append(loadErrors, discoverErr.Error())
		}
		for _, filePath := range discovered {
			if _, ok := seen[filePath]; ok {
				continue
			}
			seen[filePath] = struct{}{}
			files = append(files, filePath)
		}
	}

	sort.Strings(files)
	nextPrompts := make(map[string]Prompt)

	for _, filePath := range files {
		prompt, err := PromptFromSkillFile(filePath)
		if err != nil {
			loadErrors = append(loadErrors, err.Error())
			continue
		}
		key := promptKey(prompt.Name)
		if key == "" {
			loadErrors = append(loadErrors, "invalid prompt name")
			continue
		}
		if _, ok := nextPrompts[key]; ok {
			loadErrors = append(loadErrors, fmt.Sprintf("duplicate prompt name %q", prompt.Name))
			continue
		}
		nextPrompts[key] = normalizePrompt(prompt)
	}

	r.mu.Lock()
	r.prompts = nextPrompts
	r.loadErrors = append([]string(nil), loadErrors...)
	r.mu.Unlock()

	if len(loadErrors) == 0 {
		return nil
	}
	return errors.New(strings.Join(loadErrors, "; "))
}

func discoverSkillFiles(rawPath string) ([]string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return nil, nil
	}

	path = expandUser(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat skill path %s: %w", filepath.Clean(path), err)
	}

	results := make([]string, 0)
	if !info.IsDir() {
		if strings.EqualFold(filepath.Base(path), "SKILL.md") {
			results = append(results, filepath.Clean(path))
		}
		return results, nil
	}

	walkErr := filepath.WalkDir(path, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), "SKILL.md") {
			results = append(results, filepath.Clean(current))
		}
		return nil
	})
	if walkErr != nil {
		return results, fmt.Errorf("walk skill path %s: %w", filepath.Clean(path), walkErr)
	}
	return results, nil
}

func expandUser(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// PromptFromSkillFile converts one SKILL.md into a prompt definition.
func PromptFromSkillFile(path string) (Prompt, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Prompt{}, fmt.Errorf("read skill file %s: %w", path, err)
	}

	frontmatter, body := parseFrontmatterAndBody(string(content))
	name := firstNonEmpty(frontmatter["name"], filepath.Base(filepath.Dir(path)))
	title := strings.TrimSpace(frontmatter["title"])
	description := strings.TrimSpace(frontmatter["description"])
	if description == "" {
		description = fmt.Sprintf("Prompt loaded from %s", filepath.Base(filepath.Dir(path)))
	}

	template := strings.TrimSpace(body)
	if template == "" {
		template = fmt.Sprintf("Use skill %s to complete the task.", name)
	}

	return Prompt{
		Name:        name,
		Title:       title,
		Description: description,
		Arguments:   extractPromptArguments(template),
		Template:    template,
		SourcePath:  filepath.Clean(path),
	}, nil
}

func parseFrontmatterAndBody(raw string) (map[string]string, string) {
	trimmed := strings.TrimPrefix(raw, "\ufeff")
	normalized := normalizeLineEndings(trimmed)
	if !strings.HasPrefix(normalized, "---") {
		return map[string]string{}, raw
	}

	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return map[string]string{}, raw
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return map[string]string{}, raw
	}

	frontmatter := make(map[string]string)
	for _, line := range lines[1:end] {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}
		parts := strings.SplitN(trimmedLine, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")
		frontmatter[key] = value
	}

	body := strings.Join(lines[end+1:], "\n")
	return frontmatter, body
}

func normalizeLineEndings(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	return strings.ReplaceAll(input, "\r", "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func promptKey(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}

func normalizePrompt(prompt Prompt) Prompt {
	prompt.Name = strings.TrimSpace(prompt.Name)
	prompt.Title = strings.TrimSpace(prompt.Title)
	prompt.Description = strings.TrimSpace(prompt.Description)
	prompt.Template = strings.TrimSpace(prompt.Template)
	prompt.Arguments = normalizePromptArguments(prompt.Arguments)
	return prompt
}

func normalizePromptArguments(args []PromptArgument) []PromptArgument {
	if len(args) == 0 {
		return nil
	}

	normalized := make([]PromptArgument, 0, len(args))
	seen := make(map[string]struct{}, len(args))
	for _, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		// MCP prompt argument names are case-sensitive in strict mode.
		// Only exact duplicates are collapsed.
		key := name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, PromptArgument{
			Name:     name,
			Required: arg.Required,
		})
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Name < normalized[j].Name
	})
	return normalized
}

func extractPromptArguments(template string) []PromptArgument {
	matches := promptArgumentPattern.FindAllStringSubmatch(template, -1)
	if len(matches) == 0 {
		return nil
	}

	args := make([]PromptArgument, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		key := strings.TrimSpace(match[1])
		if key == "" {
			continue
		}
		args = append(args, PromptArgument{Name: key, Required: false})
	}
	return normalizePromptArguments(args)
}
