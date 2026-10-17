package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/opener"
)

func fmtType(v any) string { return fmt.Sprintf("%T", v) }

func TestMapShowsRoomsAndPups(t *testing.T) {
	h := newHarness(t, opener.Print)
	h.contains("kennel", "🗺  Map", "The Yard", "Lobby", "Firehouse", "Helping Out", "Courtroom", "Under the Rug", "Dumpster",
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
