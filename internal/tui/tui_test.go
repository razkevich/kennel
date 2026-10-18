package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/opener"
)

func fmtType(v any) string { return fmt.Sprintf("%T", v) }

func TestMapShowsRoomsAndPups(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.contains("kennel", "🧭 Map", "The Yard", "Lobby", "Firehouse", "Helping Out", "Courtroom", "Under the Rug", "Dumpster",
		"CHECKOUT-V2", "FLAKY-CI", "❗1 needs you", "⚡2 working")
	// The hover card describes the selected pup.
	h.contains("COST-REPORT", "● working")
	if got := h.selectedTitle(); got != "COST-REPORT" {
		t.Fatalf("selected %q", got)
	}
}

func TestMapWalkAndFloors(t *testing.T) {
	h := newHarness(t, opener.Print)
	first := h.selectedTitle()
	h.key("right")
	if h.selectedTitle() == first {
		t.Fatal("right arrow should move to the next pup")
	}
	h.key("left")
	if h.selectedTitle() != first {
		t.Fatalf("left should come back to %q, got %q", first, h.selectedTitle())
	}
	h.key("tab")
	if h.selectedTitle() == first {
		t.Fatal("tab should jump to the next room")
	}
	h.key("F")
	h.contains("📁 atlas", "📁 infra", "📁 mobile")
	h.key("F")
	h.contains("Strays", "No kennels yet")
}

func TestClickPupOnMapSelectsAndDoubleClickOpens(t *testing.T) {
	h := newHarness(t, opener.Herdr)
	target := h.pup("PG-UPGRADE")
	h.click(ZonePup(target.ID))
	if h.selectedTitle() != "PG-UPGRADE" {
		t.Fatalf("click selected %q", h.selectedTitle())
	}
	h.click(ZonePup(target.ID))
	calls := strings.Join(h.rec.Calls(), "\n")
	if !strings.Contains(calls, "herdr tab create --cwd /Users/demo/code/infra --focus --label PG-UPGRADE") ||
		!strings.Contains(calls, "herdr agent start pg-upgrade-") || !strings.Contains(calls, "--resume "+target.ID) {
		t.Fatalf("double click should resume in herdr, calls:\n%s", calls)
	}
}

func TestTabsByKeyAndClick(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("2")
	h.contains("SMART KENNELS", "PROJECTS", "YOUR KENNELS", "All Pups")
	h.click(ZoneTab(TabJobs))
	h.contains("BACKGROUND AGENTS", "SCHEDULED TASKS", "*/30 * * * *", "14 of 22 services done")
	h.key("5")
	h.contains("PG16 cutover runbook", "Incident 0612 timeline")
	h.key("6")
	h.contains("Gear", "linear", "postgres", "deploy-checklist", "old-jira", "reviewer", "standup")
	h.key("right")
	h.contains("frontend-design@claude-plugins-official", "superpowers:brainstorming")
}

func TestSearchByNameAndContent(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("/")
	h.typeText("checkout")
	h.send(searchTickMsg{seq: h.model().searchSeq})
	if h.selectedTitle() != "CHECKOUT-V2" {
		t.Fatalf("top hit %q", h.selectedTitle())
	}
	h.key("esc")
	h.key("/")
	h.typeText("paymentelement")
	h.send(searchTickMsg{seq: h.model().searchSeq})
	h.contains("CHECKOUT-V2", "PaymentElement")
	if n := len(h.model().results); n != 1 {
		t.Fatalf("content search should find exactly one pup, got %d", n)
	}
	h.key("esc")
	h.key("/")
	h.typeText("is:live repo:infra")
	got := map[string]bool{}
	for _, r := range h.model().results {
		got[r.Pup.Title()] = true
	}
	if len(got) != 2 || !got["COST-REPORT"] || !got["PG-UPGRADE"] {
		t.Fatalf("filters: %v", got)
	}
}

func TestSidebarScopeClick(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("2")
	h.click(ZoneScope("smart:helping"))
	h.contains("Helping Out", "ask from Priya", "ask from Marco")
	if n := len(h.model().results); n != 2 {
		t.Fatalf("helping out has %d", n)
	}
	h.click(ZoneScope("smart:fire"))
	h.contains("INCIDENT-0612")
}

func TestStarRenameMoveArchive(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("2")
	h.key("/")
	h.typeText("dark-mode")
	h.key("enter")
	p := h.pup("DARK-MODE")

	h.key("s")
	if !h.app.Pup(p.ID).Meta.Starred {
		t.Fatal("s should star")
	}
	h.key("1")
	h.contains("Best in Show 1")

	h.key("2", "r")
	h.contains("rename DARK-MODE")
	for range len("DARK-MODE") {
		h.send(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	h.typeText("NIGHT-OWL")
	h.key("enter")
	if got := h.app.Pup(p.ID).Title(); got != "NIGHT-OWL" {
		t.Fatalf("title %q", got)
	}
	if !strings.Contains(readFile(t, p.Path), `"customTitle":"NIGHT-OWL"`) {
		t.Fatal("rename of an asleep pup should be written to its transcript")
	}

	h.key("K")
	h.typeText("Mobile | match=night")
	h.key("enter")
	if k := h.app.Pup(p.ID).Kennel; k == nil || k.Name != "Mobile" {
		t.Fatalf("rule should adopt the pup, kennel=%v", k)
	}
	h.key("m")
	h.contains("move NIGHT-OWL to")
	h.typeText("stray")
	h.key("enter")
	if h.app.Pup(p.ID).Meta.KennelID != 0 {
		t.Fatal("moved to strays")
	}

	h.key("x")
	if !h.app.Pup(p.ID).Meta.Archived {
		t.Fatal("x should archive")
	}
}

func TestLiveSessionFocusesHerdrPaneAndQuickReply(t *testing.T) {
	h := newHarness(t, opener.Herdr)
	flaky := h.pup("FLAKY-CI")
	h.rec.agents = `{"result":{"agents":[{"agent":"claude","agent_status":"blocked","pane_id":"w3:p2","cwd":"/Users/demo/code/atlas",
		"agent_session":{"value":"` + flaky.ID + `"}}]}}`
	h.send(liveMsg{transitions: h.app.RefreshLive(true)})
	if h.app.Pup(flaky.ID).Pane != "w3:p2" || h.app.Pup(flaky.ID).Status != claude.Waiting {
		t.Fatalf("herdr state not applied: %+v", h.app.Pup(flaky.ID))
	}
	h.key("u")
	if h.selectedTitle() != "FLAKY-CI" {
		t.Fatalf("u should jump to the pup that needs you, got %q", h.selectedTitle())
	}
	h.key("enter")
	calls := h.rec.Calls()
	if calls[len(calls)-1] != "herdr agent focus w3:p2" {
		t.Fatalf("expected focus, got %v", calls)
	}
	h.key("t")
	h.typeText("yes apply both")
	h.key("enter")
	calls = h.rec.Calls()
	if calls[len(calls)-1] != "herdr agent prompt w3:p2 yes apply both" {
		t.Fatalf("quick reply, got %v", calls[len(calls)-1])
	}
}

func TestAskShowsAnswerAndJumpsToSource(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("a")
	h.typeText("why did the payment form flicker")
	h.key("enter")
	m := h.model()
	if !m.askEv.Done {
		t.Fatalf("ask not finished: %+v", m.askEv)
	}
	h.contains("memoizing options fixed it [1]", "sources", "[1] CHECKOUT-V2")
	h.key("1")
	if h.model().tab != TabPups || h.selectedTitle() != "CHECKOUT-V2" {
		t.Fatalf("1 should jump to the cited pup, tab=%d sel=%q", h.model().tab, h.selectedTitle())
	}
}

func TestTranscriptViewerSearch(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("2")
	h.key("/")
	h.typeText("pg-upgrade")
	h.key("enter", "v")
	h.contains("📜 PG-UPGRADE", "plan the postgres 15 -> 16 upgrade", "you", "▼•ᴥ•▼")
	h.key("/")
	h.typeText("pgvector")
	h.key("enter")
	h.contains("match 1/")
	h.key("esc")
	h.lacks("📜 PG-UPGRADE")
}

func TestCopyAndOpenLinks(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("2", "/")
	h.typeText("checkout-v2")
	h.key("enter", "y", "Y", "p")
	p := h.pup("CHECKOUT-V2")
	if len(h.clip) != 2 || h.clip[0] != p.ID || h.clip[1] != "cd /Users/demo/code/atlas && claude --resume "+p.ID {
		t.Fatalf("clipboard %v", h.clip)
	}
	if len(h.urls) != 1 || !strings.HasSuffix(h.urls[0], "/pull/412") {
		t.Fatalf("urls %v", h.urls)
	}
}

func TestSwitcherJumpsToPup(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("ctrl+k")
	h.contains("jump to")
	h.typeText("incident")
	h.key("enter")
	if h.model().tab != TabPups || h.selectedTitle() != "INCIDENT-0612" {
		t.Fatalf("switcher: tab=%d sel=%q", h.model().tab, h.selectedTitle())
	}
}

func TestGearToggleSkill(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("6")
	m := h.model()
	idx := -1
	for i, it := range m.gearItems() {
		if it.Name == "weekly-report" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("weekly-report skill not listed")
	}
	for range idx {
		h.key("down")
	}
	h.key("space")
	h.contains("disable skill weekly-report?")
	h.key("y")
	for _, it := range h.model().gearItems() {
		if it.Name == "weekly-report" && it.Enabled {
			t.Fatal("skill should now be disabled")
		}
	}
}

func TestHelpAndSmallWindow(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("?")
	h.contains("good dog commands", "jump anywhere")
	h.key("x")
	h.lacks("good dog commands")
	h.send(tea.WindowSizeMsg{Width: 30, Height: 8})
	h.contains("bigger window")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 30})
	h.key("2")
	h.contains("press / to search")
}

func TestThingsOnMapAndInJobs(t *testing.T) {
	h := newHarness(t, opener.Herdr)
	h.contains("Workshop", "atlas dev", "prod pods", "api logs", "tf plan", "btop")

	// Click the running dev server: selects it, card shows its command.
	devKey := ""
	stopped := ""
	for _, th := range h.app.Things() {
		switch th.Name {
		case "atlas dev":
			devKey = th.Key()
			if !th.Running || th.Tab != "w1:t2" {
				t.Fatalf("dev server should be running in w1:t2: %+v", th)
			}
		case "tf plan":
			stopped = th.Key()
			if th.Running {
				t.Fatal("tf plan should be stopped")
			}
		}
	}
	h.click(ZoneThing(devKey))
	h.contains("$ pnpm dev", "running in herdr w1:p2")
	h.key("enter")
	if c := h.rec.Calls(); c[len(c)-1] != "herdr tab focus w1:t2" {
		t.Fatalf("enter on running thing should focus its tab: %v", c)
	}
	h.key("X")
	h.contains("stop atlas dev?")
	h.key("y")
	if c := h.rec.Calls(); c[len(c)-1] != "herdr pane send-keys w1:p2 ctrl+c" {
		t.Fatalf("stop: %v", c)
	}

	h.key("4")
	h.contains("THINGS", "found in herdr (S to save)", "$ terraform plan -out tf.plan")
	rows := h.model().jobRows()
	for i, r := range rows {
		if r.thing == stopped {
			for range i {
				h.key("down")
			}
		}
	}
	h.key("enter")
	calls := strings.Join(h.rec.Calls(), "\n")
	if !strings.Contains(calls, "herdr tab create --cwd /Users/demo/code/infra --focus --label tf plan") ||
		!strings.Contains(calls, "herdr pane run w1:p9 terraform plan -out tf.plan") {
		t.Fatalf("starting a stopped thing:\n%s", calls)
	}

	h.key("N")
	h.typeText("docs | mkdocs serve | ~/code/atlas")
	h.key("enter")
	found := false
	for _, th := range h.app.Things() {
		if th.Name == "docs" && th.Command == "mkdocs serve" && th.Kind == "server" {
			found = true
		}
	}
	if !found {
		t.Fatal("new thing not saved")
	}
}

func TestBuildRoomFromTheTUI(t *testing.T) {
	h := newHarness(t, opener.Print)
	if h.selectedTitle() != "COST-REPORT" {
		t.Fatalf("start in the yard, got %q", h.selectedTitle())
	}
	h.key("B")
	h.contains("build 🌳 The Yard", "template · 🚨 On-call room", "shape · 🏢 Office")
	h.typeText("add")
	h.contains("add 🚪 Spawn door", "add 🔔 Bell", "add 🌀 Portal")
	for range 3 {
		h.send(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	h.typeText("on-call")
	h.key("enter")
	st := h.app.Style("yard")
	if st.Shape != "office" || st.Floor != "tiles" || len(st.OfKind("door")) != 1 {
		t.Fatalf("template not applied: %+v", st)
	}
	h.contains("The Yard 6", "🔔🚪", "🧯☕📋")

	h.key("B")
	h.typeText("add 📌")
	h.key("enter")
	h.contains("Pin board")
	h.typeText("text=runbook lives in docs/oncall.md")
	h.key("enter")
	h.contains("📌 runbook lives in docs/oncall.md")

	h.key("n")
	if !strings.Contains(h.screen(), "claude -n ONCALL") {
		t.Fatalf("door should spawn ONCALL:\n%s", h.screen())
	}

	h.key("B")
	h.typeText("reset")
	h.key("enter")
	if len(h.app.Style("yard").Blocks) != 0 {
		t.Fatal("reset should clear the room")
	}
}

func TestSelectionSticksWhenListReorders(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("2", "down", "down", "down")
	want := h.selectedTitle()
	p := h.pup(want)
	f, _ := os.OpenFile(p.Path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"user","sessionId":"` + p.ID + `","timestamp":"2026-10-18T12:00:30Z","message":{"role":"user","content":"one more thing"}}` + "\n")
	f.Close()
	h.app.Sync(context.Background(), nil)
	h.send(liveMsg{})
	if got := h.selectedTitle(); got != want {
		t.Fatalf("selection moved from %q to %q", want, got)
	}
	// Pups that need you float to the top; right after them comes the newest.
	if h.model().cursor != 1 {
		t.Fatalf("%s is now newest and should follow FLAKY-CI, cursor=%d", want, h.model().cursor)
	}
}

func TestSuggestName(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("2", "/")
	h.typeText("staging dry run")
	h.send(searchTickMsg{seq: h.model().searchSeq})
	h.key("enter", "T")
	h.contains("suggested name for PG-UPGRADE", "PG-UPGRADE")
	if v := h.model().modal.input.Value(); v != "PG-UPGRADE" {
		t.Fatalf("suggestion %q", v)
	}
}

func TestCloneMergeDistill(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.key("2", "/")
	h.typeText("pg-upgrade")
	h.key("enter", "C")
	h.contains("clone PG-UPGRADE into", "same project", "mobile")
	h.typeText("mobile")
	h.key("enter")
	h.contains("PG-UPGRADE-CLONE")
	if h.selectedTitle() != "PG-UPGRADE-CLONE" {
		t.Fatalf("clone should be selected, got %q", h.selectedTitle())
	}

	h.key("esc", "/")
	h.typeText("dark-mode")
	h.key("enter", "M")
	h.typeText("ios-crash")
	h.key("enter")
	h.contains("name the merged pup")
	h.key("enter")
	if !strings.Contains(h.screen(), "claude -n DARK-MODE+IOS-CRASH") {
		t.Fatalf("merge should start a new pup:\n%s", h.screen())
	}

	h.key("Z", "enter")
	if !strings.Contains(h.screen(), "claude -n DARK-MODE-FRESH") {
		t.Fatalf("distill:\n%s", h.screen())
	}
}
