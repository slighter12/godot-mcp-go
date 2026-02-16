package promptcatalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Prompt represents one prompt exposed through MCP prompt endpoints.
type Prompt struct {
	Name        string
	Description string
	Template    string
	SourcePath  string
}

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
	prompt.Description = strings.TrimSpace(prompt.Description)
	prompt.Template = strings.TrimSpace(prompt.Template)
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

func (r *Registry) recordError(err error) {
	if r == nil || err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadErrors = append(r.loadErrors, err.Error())
}

// LoadFromPaths discovers SKILL.md files and registers prompt metadata.
func (r *Registry) LoadFromPaths(paths []string) error {
	if !r.Enabled() {
		return nil
	}
	r.reset()

	files := make([]string, 0)
	seen := make(map[string]struct{})
	for _, rawPath := range paths {
		for _, filePath := range discoverSkillFiles(rawPath) {
			if _, ok := seen[filePath]; ok {
				continue
			}
			seen[filePath] = struct{}{}
			files = append(files, filePath)
		}
	}

	sort.Strings(files)

	for _, filePath := range files {
		prompt, err := PromptFromSkillFile(filePath)
		if err != nil {
			r.recordError(err)
			continue
		}
		key := promptKey(prompt.Name)
		if key == "" {
			r.recordError(fmt.Errorf("invalid prompt name from %s", filePath))
			continue
		}
		if existing, ok := r.lookupPromptByKey(key); ok {
			r.recordError(fmt.Errorf("duplicate prompt name %q in %s (already defined in %s)", prompt.Name, prompt.SourcePath, existing.SourcePath))
			continue
		}
		r.RegisterPrompt(prompt)
	}

	errorsFound := r.LoadErrors()
	if len(errorsFound) == 0 {
		return nil
	}
	return errors.New(strings.Join(errorsFound, "; "))
}

func (r *Registry) reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prompts = make(map[string]Prompt)
	r.loadErrors = nil
}

func discoverSkillFiles(rawPath string) []string {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return nil
	}

	path = expandUser(path)
	info, err := os.Stat(path)
	if err != nil {
		return []string{filepath.Clean(path)}
	}

	results := make([]string, 0)
	if !info.IsDir() {
		if strings.EqualFold(filepath.Base(path), "SKILL.md") {
			results = append(results, filepath.Clean(path))
		}
		return results
	}

	_ = filepath.WalkDir(path, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), "SKILL.md") {
			results = append(results, filepath.Clean(current))
		}
		return nil
	})

	return results
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
		Description: description,
		Template:    template,
		SourcePath:  filepath.Clean(path),
	}, nil
}

func parseFrontmatterAndBody(raw string) (map[string]string, string) {
	trimmed := strings.TrimPrefix(raw, "\ufeff")
	if !strings.HasPrefix(trimmed, "---") {
		return map[string]string{}, raw
	}

	lines := strings.Split(trimmed, "\n")
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

func (r *Registry) lookupPromptByKey(key string) (Prompt, bool) {
	if r == nil || key == "" {
		return Prompt{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	prompt, ok := r.prompts[key]
	return prompt, ok
}
