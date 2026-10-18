package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/razkevich/kennel/internal/blocks"
	"github.com/razkevich/kennel/internal/floor"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/store"
)

// currentRoomKey is the room being looked at: the selected tile's room on the
// map, or the sidebar scope in the list.
func (m Model) currentRoomKey() string {
	if m.tab == TabMap {
		sel := mapSelection(m.mapSel, m.mapGeometry())
		for _, l := range m.mapGeometry() {
			for _, it := range l.items {
				if it.id() == sel {
					return l.key
				}
			}
		}
		if rs := m.rooms(); len(rs) > 0 {
			return rs[0].key
		}
		return ""
	}
	key := m.scope().key
	if k, ok := strings.CutPrefix(key, "smart:"); ok {
		return k
	}
	return key
}

func (m *Model) buildModal() *modal {
	key := m.currentRoomKey()
	room, ok := floor.FindRoom(m.app, key)
	if !ok {
		return nil
	}
	st := room.Style
	var choices []choice
	for _, t := range blocks.Templates {
		choices = append(choices, choice{label: "🎨 template · " + t.Icon + " " + t.Name, value: "tpl:" + t.Key})
	}
	for _, sh := range blocks.Shapes {
		on := ""
		if st.Shape == sh.Key {
			on = " ✓"
		}
		choices = append(choices, choice{label: "📐 shape · " + sh.Icon + " " + sh.Name + on, detail: sh.Blurb, value: "shape:" + sh.Key})
	}
	for _, f := range blocks.Floors {
		on := ""
		if st.Floor == f.Key {
			on = " ✓"
		}
		choices = append(choices, choice{label: "🟫 floor · " + f.Name + on, value: "floor:" + f.Key})
	}
	for _, w := range blocks.Walls {
		on := ""
		if st.Wall == w.Key {
			on = " ✓"
		}
		choices = append(choices, choice{label: "🧱 walls · " + w.Name + on, value: "wall:" + w.Key})
	}
	for _, k := range blocks.Kinds {
		choices = append(choices, choice{label: "➕ add " + k.Icon + " " + k.Name, detail: k.Blurb, value: "add:" + k.Key})
	}
	for i, b := range st.Blocks {
		k := blocks.KindByKey(b.Kind)
		if k == nil {
			continue
		}
		choices = append(choices,
			choice{label: "✎ edit " + k.Icon + " " + k.Name, detail: configText(b), value: fmt.Sprintf("edit:%d", i)},
			choice{label: "✕ remove " + k.Icon + " " + k.Name, detail: configText(b), value: fmt.Sprintf("del:%d", i)})
	}
	choices = append(choices, choice{label: "↺ reset this room", value: "reset"})
	return pickerModal("🔨 build "+room.Icon+" "+room.Title, choices, func(m *Model, _, value string) tea.Cmd {
		return m.applyBuild(key, value)
	})
}

func configText(b store.Block) string {
	k := blocks.KindByKey(b.Kind)
	if k == nil {
		return ""
	}
	var parts []string
	for _, f := range k.Fields {
		parts = append(parts, f.Key+"="+b.Get(f.Key))
	}
	return strings.Join(parts, " | ")
}

func parseConfig(text string) map[string]string {
	cfg := map[string]string{}
	for _, part := range strings.Split(text, "|") {
		if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && strings.TrimSpace(v) != "" {
			cfg[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return cfg
}

func (m *Model) applyBuild(key, value string) tea.Cmd {
	st := m.app.Style(key)
	kind, arg, _ := strings.Cut(value, ":")
	save := func(msg string) tea.Cmd {
		if err := m.app.SaveStyle(key, st); err != nil {
			return toast(err.Error(), true)
		}
		m.rebuildScopes()
		return tea.Batch(m.refreshResults(), toast("🔨 "+msg, false))
	}
	switch kind {
	case "tpl":
		for _, t := range blocks.Templates {
			if t.Key == arg {
				if err := floor.ApplyTemplate(m.app, key, t); err != nil {
					return toast(err.Error(), true)
				}
				return toast("🔨 "+t.Icon+" "+t.Name+" built", false)
			}
		}
	case "shape":
		st.Shape = arg
		return save("shape: " + arg)
	case "floor":
		st.Floor = arg
		return save("floor: " + arg)
	case "wall":
		st.Wall = arg
		return save("walls: " + arg)
	case "add":
		k := blocks.KindByKey(arg)
		var hints []string
		for _, f := range k.Fields {
			hints = append(hints, f.Key+"="+f.Placeholder)
		}
		m.modal = textModal(k.Icon+" "+k.Name, k.Blurb+"  ·  key=value | key=value", "", strings.Join(hints, " | "), func(m *Model, text, _ string) tea.Cmd {
			st := m.app.Style(key)
			st.Blocks = append(st.Blocks, store.Block{Kind: k.Key, X: 0.85, Y: 0.3 + 0.15*float64(len(st.Blocks)%4), Config: parseConfig(text)})
			if err := m.app.SaveStyle(key, st); err != nil {
				return toast(err.Error(), true)
			}
			m.rebuildScopes()
			return tea.Batch(m.refreshResults(), toast("🔨 added "+k.Icon+" "+k.Name, false))
		})
		return nil
	case "edit":
		var i int
		fmt.Sscan(arg, &i)
		if i >= len(st.Blocks) {
			return nil
		}
		b := st.Blocks[i]
		k := blocks.KindByKey(b.Kind)
		m.modal = textModal("✎ "+k.Icon+" "+k.Name, "key=value | key=value", configText(b), "", func(m *Model, text, _ string) tea.Cmd {
			st := m.app.Style(key)
			if i < len(st.Blocks) {
				st.Blocks[i].Config = parseConfig(text)
			}
			if err := m.app.SaveStyle(key, st); err != nil {
				return toast(err.Error(), true)
			}
			m.rebuildScopes()
			return m.refreshResults()
		})
		return nil
	case "del":
		var i int
		fmt.Sscan(arg, &i)
		if i < len(st.Blocks) {
			st.Blocks = append(st.Blocks[:i], st.Blocks[i+1:]...)
		}
		return save("removed block")
	case "reset":
		if err := m.app.ResetStyle(key); err != nil {
			return toast(err.Error(), true)
		}
		m.rebuildScopes()
		return tea.Batch(m.refreshResults(), toast("↺ room reset", false))
	}
	return nil
}

// spawnFromDoor starts a pup through the current room's spawn door, if it has one.
func (m *Model) spawnFromDoor() (tea.Cmd, bool) {
	room, ok := floor.FindRoom(m.app, m.currentRoomKey())
	if !ok {
		return nil, false
	}
	doors := room.Style.OfKind(blocks.Door)
	if len(doors) == 0 {
		return nil, false
	}
	t, err := floor.Spawn(m.app, room, doors[0])
	if err != nil {
		return toast(err.Error(), true), true
	}
	if err := m.app.CheckCwd(t.Cwd); err != nil {
		return toast(err.Error(), true), true
	}
	if opener.Resolve(m.app.Cfg.Opener) == opener.Inline {
		return tea.ExecProcess(opener.InlineCmd(t), func(err error) tea.Msg {
			return openedMsg{text: "back in the kennel 🐾", err: err}
		}), true
	}
	o := m.app.Opener()
	return func() tea.Msg {
		text, err := o.Open(t)
		return openedMsg{text: "🚪 " + text, err: err}
	}, true
}
