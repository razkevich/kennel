package gear

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func find(scopes []Scope, kind Kind, name string) *Item {
	for _, s := range scopes {
		for i := range s.Items {
			if s.Items[i].Kind == kind && s.Items[i].Name == name {
				return &s.Items[i]
			}
		}
	}
	return nil
}

func TestInventoryAndToggles(t *testing.T) {
	root := t.TempDir()
	cd := filepath.Join(root, "claude")
	proj := filepath.Join(root, "proj")
	write(t, filepath.Join(root, ".claude.json"), `{"mcpServers":{"linear":{"type":"http","url":"https://mcp.linear.app/mcp"}}}`)
	settings := "{\n  \"zeta\": 1,\n  \"enabledPlugins\": {\n    \"superpowers@dev\": true\n  },\n  \"hooks\": {\"Stop\": [{\"hooks\": [{\"command\": \"say done\"}]}]}\n}\n"
	write(t, filepath.Join(cd, "settings.json"), settings)
	write(t, filepath.Join(cd, "skills", "grafatui", "SKILL.md"), "---\nname: grafatui\ndescription: Charts in the terminal\n---\n")
	write(t, filepath.Join(cd, "plugins", "cache", "dev", "superpowers", "1.0", "skills", "brainstorming", "SKILL.md"), "---\ndescription: ideas\n---\n")
	write(t, filepath.Join(proj, ".mcp.json"), `{"mcpServers":{"pg":{"command":"pg-mcp","args":["--ro"]}}}`)
	write(t, filepath.Join(proj, ".claude", "agents", "reviewer.md"), "---\ndescription: reviews code\n---\n")

	scopes := Inventory(cd, []string{proj})
	if len(scopes) != 3 {
		t.Fatalf("scopes=%d", len(scopes))
	}
	if it := find(scopes, MCP, "linear"); it == nil || it.Detail != "http https://mcp.linear.app/mcp" || it.Toggle != nil {
		t.Fatalf("user mcp: %+v", it)
	}
	if it := find(scopes, Skill, "superpowers:brainstorming"); it == nil || !it.Enabled {
		t.Fatalf("plugin skill: %+v", it)
	}
	if it := find(scopes, Agent, "reviewer"); it == nil || it.Detail != "reviews code" {
		t.Fatalf("agent: %+v", it)
	}
	if it := find(scopes, Hook, "Stop"); it == nil || it.Detail != "say done" {
		t.Fatalf("hook: %+v", it)
	}

	sk := find(scopes, Skill, "grafatui")
	if sk == nil || !sk.Enabled || sk.Detail != "Charts in the terminal" {
		t.Fatalf("skill: %+v", sk)
	}
	if err := sk.Toggle(); err != nil {
		t.Fatal(err)
	}
	if sk := find(Inventory(cd, nil), Skill, "grafatui"); sk == nil || sk.Enabled {
		t.Fatalf("skill should be disabled: %+v", sk)
	}

	pl := find(scopes, Plugin, "superpowers@dev")
	if err := pl.Toggle(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(cd, "settings.json"))
	if string(b) != strings.Replace(settings, "true", "false", 1) {
		t.Fatalf("settings rewritten unexpectedly:\n%s", b)
	}

	mcp := find(scopes, MCP, "pg")
	if !mcp.Enabled {
		t.Fatal("pg should start enabled")
	}
	mcp.Toggle()
	if find(Inventory(cd, []string{proj}), MCP, "pg").Enabled {
		t.Fatal("pg should be disabled")
	}
	mcp.Toggle()
	if !find(Inventory(cd, []string{proj}), MCP, "pg").Enabled {
		t.Fatal("pg should be re-enabled")
	}
}
