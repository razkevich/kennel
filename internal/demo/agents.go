package demo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// CodexDir and CursorDir are where Generate puts the other agents' fixtures,
// next to the synthetic ~/.claude.
func CodexDir(claudeDir string) string  { return filepath.Join(filepath.Dir(claudeDir), "codex") }
func CursorDir(claudeDir string) string { return filepath.Join(filepath.Dir(claudeDir), "cursor") }

type codexSpec struct {
	id, cwd, branch string
	ago             time.Duration
	turns           [][3]string // user, assistant, shell command
}

func codexSpecs() []codexSpec {
	return []codexSpec{
		{"0199c1a2-7e3b-7d40-9a51-6f2b1c0e4a11", "/Users/demo/code/atlas", "perf/csv-export", 3 * time.Hour, [][3]string{
			{"speed up the CSV export endpoint, it times out for large tenants", "The export loads every row into memory before writing. Streaming rows with a cursor and flushing every 500 rows keeps memory flat and finishes the 2M-row tenant in 38s.", "rg -n exportCsv src"},
			{"add a progress header so the UI can show a bar", "Added `X-Export-Rows` as a trailer and a polling endpoint `/exports/:id/progress`.", ""},
		}},
		{"0199c3f0-11aa-7c2e-8b07-2d4e9f6a7b22", "/Users/demo/code/infra", "feat/redis-module", 50 * time.Hour, [][3]string{
			{"write a terraform module for the new redis cluster with replicas in two zones", "Created `modules/redis` with a primary, two replicas across zones, parameter group with `maxmemory-policy allkeys-lru`, and alarms on evictions.", "terraform validate"},
		}},
	}
}

func writeCodex(dir string, now time.Time) error {
	for _, sp := range codexSpecs() {
		start := now.Add(-sp.ago - 40*time.Minute)
		day := filepath.Join(dir, "sessions", start.Format("2006"), start.Format("01"), start.Format("02"))
		if err := os.MkdirAll(day, 0o755); err != nil {
			return err
		}
		var lines []byte
		add := func(at time.Time, typ string, payload any) {
			b, _ := json.Marshal(map[string]any{"timestamp": at.UTC().Format(time.RFC3339Nano), "type": typ, "payload": payload})
			lines = append(append(lines, b...), '\n')
		}
		add(start, "session_meta", map[string]any{"id": sp.id, "cwd": sp.cwd, "originator": "codex_cli_rs", "cli_version": "0.155.0",
			"git": map[string]any{"branch": sp.branch}})
		add(start, "turn_context", map[string]any{"cwd": sp.cwd, "model": "gpt-5-codex"})
		add(start, "response_item", map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "<environment_context>cwd: " + sp.cwd + "</environment_context>"}}})
		for i, t := range sp.turns {
			at := start.Add(time.Duration(i*15) * time.Minute)
			add(at, "response_item", map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": t[0]}}})
			if t[2] != "" {
				args, _ := json.Marshal(map[string]any{"command": []string{"bash", "-lc", t[2]}})
				add(at.Add(10*time.Second), "response_item", map[string]any{"type": "function_call", "name": "shell", "arguments": string(args), "call_id": fmt.Sprintf("call_%d", i)})
			}
			add(at.Add(30*time.Second), "response_item", map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": t[1]}}})
		}
		add(now.Add(-sp.ago), "event_msg", map[string]any{"type": "token_count"})
		name := fmt.Sprintf("rollout-%s-%s.jsonl", start.Format("2006-01-02T15-04-05"), sp.id)
		if err := os.WriteFile(filepath.Join(day, name), lines, 0o644); err != nil {
			return err
		}
	}
	return nil
}

type cursorSpec struct {
	id, name, folder string
	ago              time.Duration
	bubbles          []string // alternating user, assistant
}

func cursorSpecs() []cursorSpec {
	return []cursorSpec{
		{"5d3c2b1a-0f9e-4d8c-b7a6-112233445566", "Onboarding copy polish", "/Users/demo/code/atlas", 5 * time.Hour, []string{
			"tighten the onboarding tour copy, it's too wordy",
			"Shortened all four steps to one sentence each and moved the keyboard hints into tooltips.",
			"make the last step celebrate a bit more",
			"Added a confetti burst on the final step and changed the button to “Let's go!”.",
		}},
		{"6e4d3c2b-1a0f-4e9d-a8b7-223344556677", "Widget tests", "/Users/demo/code/mobile", 4 * 24 * time.Hour, []string{
			"write widget tests for the settings screen",
			"Added 12 widget tests covering toggles, the theme picker and sign-out confirmation; all pass.",
		}},
	}
}

func writeCursor(dir string, now time.Time) error {
	global := filepath.Join(dir, "globalStorage")
	if err := os.MkdirAll(global, 0o755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(global, "state.vscdb"))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS cursorDiskKV (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB)`); err != nil {
		return err
	}
	byFolder := map[string][]string{}
	for _, sp := range cursorSpecs() {
		var headers []map[string]any
		for i, text := range sp.bubbles {
			typ := 2
			if i%2 == 0 {
				typ = 1
			}
			bid := fmt.Sprintf("b%02d-%s", i, sp.id[:8])
			headers = append(headers, map[string]any{"bubbleId": bid, "type": typ})
			b, _ := json.Marshal(map[string]any{"type": typ, "text": text})
			if _, err := db.Exec(`INSERT INTO cursorDiskKV(key, value) VALUES(?, ?)`, "bubbleId:"+sp.id+":"+bid, b); err != nil {
				return err
			}
		}
		updated := now.Add(-sp.ago)
		c, _ := json.Marshal(map[string]any{"composerId": sp.id, "name": sp.name, "createdAt": updated.Add(-time.Hour).UnixMilli(),
			"lastUpdatedAt": updated.UnixMilli(), "fullConversationHeadersOnly": headers, "unifiedMode": "agent"})
		if _, err := db.Exec(`INSERT INTO cursorDiskKV(key, value) VALUES(?, ?)`, "composerData:"+sp.id, c); err != nil {
			return err
		}
		byFolder[sp.folder] = append(byFolder[sp.folder], sp.id)
	}
	i := 0
	for folder, ids := range byFolder {
		ws := filepath.Join(dir, "workspaceStorage", fmt.Sprintf("ws%02d", i))
		i++
		if err := os.MkdirAll(ws, 0o755); err != nil {
			return err
		}
		meta, _ := json.Marshal(map[string]string{"folder": "file://" + folder})
		if err := os.WriteFile(filepath.Join(ws, "workspace.json"), meta, 0o644); err != nil {
			return err
		}
		wdb, err := sql.Open("sqlite", "file:"+filepath.Join(ws, "state.vscdb"))
		if err != nil {
			return err
		}
		var all []map[string]string
		for _, id := range ids {
			all = append(all, map[string]string{"composerId": id})
		}
		val, _ := json.Marshal(map[string]any{"allComposers": all})
		_, err = wdb.Exec(`CREATE TABLE IF NOT EXISTS ItemTable (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB)`)
		if err == nil {
			_, err = wdb.Exec(`INSERT INTO ItemTable(key, value) VALUES('composer.composerData', ?)`, val)
		}
		wdb.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
