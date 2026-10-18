// Package gear inventories Claude Code extensions — MCP servers, skills, plugins,
// subagents, slash commands and hooks — at user, project and plugin level.
package gear

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Kind string

const (
	MCP     Kind = "mcp"
	Skill   Kind = "skill"
	Plugin  Kind = "plugin"
	Agent   Kind = "agent"
	Command Kind = "command"
	Hook    Kind = "hook"
)

var KindIcon = map[Kind]string{MCP: "🔌", Skill: "🦴", Plugin: "🧩", Agent: "🐕", Command: "⌘", Hook: "🪝"}

type Item struct {
	Kind    Kind
	Name    string
	Level   string
	Detail  string
	Path    string
	Enabled bool
	// Toggle is nil when kennel can't safely change this item.
	Toggle func() error
}

type Scope struct {
	Name  string
	Root  string
	Items []Item
}

// Inventory scans the user scope, every project root, and installed plugins.
func Inventory(claudeDir string, projects []string) []Scope {
	scopes := []Scope{userScope(claudeDir)}
	for _, p := range projects {
		if s := projectScope(p); len(s.Items) > 0 {
			scopes = append(scopes, s)
		}
	}
	scopes = append(scopes, pluginScope(claudeDir))
	return scopes
}

func claudeJSON(claudeDir string) string {
	return filepath.Join(filepath.Dir(claudeDir), ".claude.json")
}

func userScope(dir string) Scope {
	s := Scope{Name: "User", Root: dir}
	var cj struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	readJSON(claudeJSON(dir), &cj)
	for name, raw := range cj.MCPServers {
		s.Items = append(s.Items, Item{Kind: MCP, Name: name, Level: "user", Detail: mcpDetail(raw), Path: claudeJSON(dir), Enabled: true})
	}
	s.Items = append(s.Items, skills(filepath.Join(dir, "skills"), filepath.Join(dir, "skills-disabled"), "user")...)
	s.Items = append(s.Items, mdItems(filepath.Join(dir, "agents"), Agent, "user")...)
	s.Items = append(s.Items, mdItems(filepath.Join(dir, "commands"), Command, "user")...)
	s.Items = append(s.Items, hooks(filepath.Join(dir, "settings.json"), "user")...)
	sortItems(s.Items)
	return s
}

func projectScope(root string) Scope {
	s := Scope{Name: filepath.Base(root), Root: root}
	var mj struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	readJSON(filepath.Join(root, ".mcp.json"), &mj)
	local := filepath.Join(root, ".claude", "settings.local.json")
	var ls struct {
		Disabled []string `json:"disabledMcpjsonServers"`
	}
	readJSON(local, &ls)
	for name, raw := range mj.MCPServers {
		name := name
		enabled := !contains(ls.Disabled, name)
		s.Items = append(s.Items, Item{Kind: MCP, Name: name, Level: "project", Detail: mcpDetail(raw), Path: filepath.Join(root, ".mcp.json"),
			Enabled: enabled, Toggle: func() error { return toggleListEntry(local, "disabledMcpjsonServers", name) }})
	}
	cd := filepath.Join(root, ".claude")
	s.Items = append(s.Items, skills(filepath.Join(cd, "skills"), filepath.Join(cd, "skills-disabled"), "project")...)
	s.Items = append(s.Items, mdItems(filepath.Join(cd, "agents"), Agent, "project")...)
	s.Items = append(s.Items, mdItems(filepath.Join(cd, "commands"), Command, "project")...)
	s.Items = append(s.Items, hooks(filepath.Join(cd, "settings.json"), "project")...)
	s.Items = append(s.Items, hooks(local, "local")...)
	sortItems(s.Items)
	return s
}

func pluginScope(dir string) Scope {
	s := Scope{Name: "Plugins", Root: filepath.Join(dir, "plugins")}
	settings := filepath.Join(dir, "settings.json")
	var st struct {
		Enabled map[string]bool `json:"enabledPlugins"`
	}
	readJSON(settings, &st)
	for name, on := range st.Enabled {
		name := name
		plugin, market, _ := strings.Cut(name, "@")
		s.Items = append(s.Items, Item{Kind: Plugin, Name: name, Level: "plugin", Detail: "from " + market, Path: settings, Enabled: on,
			Toggle: func() error { return togglePlugin(settings, name) }})
		for _, sk := range pluginSkills(dir, market, plugin) {
			sk.Enabled = on
			s.Items = append(s.Items, sk)
		}
	}
	sortItems(s.Items)
	return s
}

func pluginSkills(dir, market, plugin string) []Item {
	matches, _ := filepath.Glob(filepath.Join(dir, "plugins", "cache", market, plugin, "*", "skills", "*", "SKILL.md"))
	seen := map[string]bool{}
	var out []Item
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	for _, m := range matches {
		name := filepath.Base(filepath.Dir(m))
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, Item{Kind: Skill, Name: plugin + ":" + name, Level: "plugin", Detail: frontmatter(m, "description"), Path: m})
	}
	return out
}

func skills(on, off, level string) []Item {
	var out []Item
	for _, base := range []string{on, off} {
		enabled := base == on
		entries, _ := os.ReadDir(base)
		for _, e := range entries {
			if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
				continue
			}
			name := e.Name()
			md := filepath.Join(base, name, "SKILL.md")
			if _, err := os.Stat(md); err != nil {
				continue
			}
			src := filepath.Join(base, name)
			dst := filepath.Join(off, name)
			if !enabled {
				dst = filepath.Join(on, name)
			}
			out = append(out, Item{Kind: Skill, Name: name, Level: level, Detail: frontmatter(md, "description"), Path: src, Enabled: enabled,
				Toggle: func() error { return moveDir(src, dst) }})
		}
	}
	return out
}

func mdItems(dir string, kind Kind, level string) []Item {
	var out []Item
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	for _, f := range files {
		out = append(out, Item{Kind: kind, Name: strings.TrimSuffix(filepath.Base(f), ".md"), Level: level, Detail: frontmatter(f, "description"), Path: f, Enabled: true})
	}
	return out
}

func hooks(settings, level string) []Item {
	var st struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	readJSON(settings, &st)
	var out []Item
	for event, groups := range st.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				out = append(out, Item{Kind: Hook, Name: event, Level: level, Detail: h.Command, Path: settings, Enabled: true})
			}
		}
	}
	return out
}

func mcpDetail(raw json.RawMessage) string {
	var m struct {
		Type    string   `json:"type"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
		URL     string   `json:"url"`
	}
	_ = json.Unmarshal(raw, &m)
	if m.URL != "" {
		return strings.TrimSpace(m.Type + " " + m.URL)
	}
	return strings.TrimSpace(m.Command + " " + strings.Join(m.Args, " "))
}

func frontmatter(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	inFM := false
	for i := 0; sc.Scan() && i < 60; i++ {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			if inFM {
				return ""
			}
			inFM = true
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok && inFM && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

func readJSON(path string, v any) {
	b, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(b, v)
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func sortItems(items []Item) {
	order := map[Kind]int{MCP: 0, Skill: 1, Plugin: 2, Agent: 3, Command: 4, Hook: 5}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return order[items[i].Kind] < order[items[j].Kind]
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
}

func moveDir(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists", dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// togglePlugin flips one boolean in enabledPlugins with a targeted text edit,
// so the rest of the user's settings.json (order, formatting) is untouched.
func togglePlugin(settings, name string) error {
	b, err := os.ReadFile(settings)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`("` + regexp.QuoteMeta(name) + `"\s*:\s*)(true|false)`)
	loc := re.FindSubmatchIndex(b)
	if loc == nil {
		return fmt.Errorf("plugin %s not found in %s", name, settings)
	}
	val := "true"
	if string(b[loc[4]:loc[5]]) == "true" {
		val = "false"
	}
	out := append(append(append([]byte{}, b[:loc[4]]...), val...), b[loc[5]:]...)
	return writeAtomic(settings, out)
}

// toggleListEntry adds or removes name from a string-array key in a JSON settings file.
// settings.local.json is small and machine-written, so a full rewrite is acceptable here.
func toggleListEntry(path, key, name string) error {
	m := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	var list []any
	if l, ok := m[key].([]any); ok {
		list = l
	}
	var next []any
	found := false
	for _, v := range list {
		if v == name {
			found = true
			continue
		}
		next = append(next, v)
	}
	if !found {
		next = append(next, name)
	}
	m[key] = next
	if len(next) == 0 {
		delete(m, key)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeAtomic(path, append(b, '\n'))
}

func writeAtomic(path string, b []byte) error {
	// Settings often live in a dotfiles repo behind a symlink; write through it.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp := path + ".kennel-tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
