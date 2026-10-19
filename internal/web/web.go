// Package web serves kennel's Gather-style map: a pixel-art office where your
// sessions wander around, backed by a small JSON + SSE API on localhost.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/razkevich/kennel/internal/ghostty"
	"github.com/razkevich/kennel/internal/term"
	"hash/fnv"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/ask"
	"github.com/razkevich/kennel/internal/blocks"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/floor"
	"github.com/razkevich/kennel/internal/indexer"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
	"github.com/razkevich/kennel/internal/things"
	"github.com/razkevich/kennel/internal/treats"
)

//go:embed static
var static embed.FS

type Server struct {
	App   *app.App
	Token string
	// Herdr enables polling herdr for pane state.
	Herdr bool

	mu      sync.Mutex
	version int64
	print   uint64
	// indexing is "done/total" while the first sync runs, else "".
	indexing atomic.Value
	// themes reports the categorizer's progress.
	themes atomic.Value

	subMu sync.Mutex
	subs  map[chan []byte]bool

	// Terms are the sessions running in the page's own terminal panel.
	Terms *term.Manager
}

// handoff broadcasts a launch to connected pages; the Mac app's page opens it as a tab.
func (s *Server) handoff(r opener.Run) error {
	b, _ := json.Marshal(r)
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if len(s.subs) == 0 {
		return errors.New("the Kennel app isn't connected")
	}
	for ch := range s.subs {
		select {
		case ch <- b:
		default:
		}
	}
	return nil
}

type ThemeStatus struct {
	Running  bool     `json:"running"`
	Done     int      `json:"done"`
	Total    int      `json:"total"`
	Provider string   `json:"provider"`
	Error    string   `json:"error,omitempty"`
	Themes   []string `json:"themes"`
}

func NewToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Serve runs until ctx is done.
func Serve(ctx context.Context, a *app.App, addr string, open bool, token string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if token == "" {
		token = NewToken()
	}
	s := &Server{App: a, Token: token, Herdr: !a.Cfg.Demo, Terms: term.NewManager()}
	if a.Cfg.Opener == opener.App {
		a.Cfg.Handoff = s.handoff
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go s.pump(ctx)
	url := fmt.Sprintf("http://%s/?t=%s", ln.Addr().String(), s.Token)
	fmt.Println("▼•ᴥ•▼ kennel map at", url)
	if open {
		openBrowser(url)
	}
	go func() {
		<-ctx.Done()
		s.Terms.StopAll()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Start()
	case "linux":
		_ = exec.Command("xdg-open", url).Start()
	}
}

// pump refreshes live state and the index, bumping version when anything visible changes.
func (s *Server) pump(ctx context.Context) {
	s.indexing.Store("starting")
	_, _ = s.App.Sync(ctx, func(p indexer.Progress) {
		s.indexing.Store(fmt.Sprintf("%d/%d", p.Done, p.Total))
		s.touch()
	})
	s.indexing.Store("")
	s.touch()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n++
		floor.Ring(s.App, s.App.RefreshLive(s.Herdr && n%2 == 0))
		if n%3 == 0 {
			_, _ = s.App.Sync(ctx, nil)
			floor.Sweep(s.App)
		}
		s.touch()
	}
}

func (s *Server) touch() {
	fp := s.fingerprint()
	s.mu.Lock()
	if fp != s.print {
		s.print = fp
		s.version++
	}
	s.mu.Unlock()
}

func (s *Server) fingerprint() uint64 {
	h := fnv.New64a()
	ix, _ := s.indexing.Load().(string)
	fmt.Fprintf(h, "ix%s;", ix)
	th, _ := s.themes.Load().(ThemeStatus)
	fmt.Fprintf(h, "th%v%d%s;", th.Running, th.Done, th.Error)
	fmt.Fprintf(h, "ap%s;", s.App.Store.Get("web_appearance"))
	for _, p := range s.App.Pups() {
		fmt.Fprintf(h, "t%s", p.Meta.Theme)
	}
	for _, p := range s.App.Pups() {
		fmt.Fprintf(h, "%s|%d|%d|%s|%v|%v|%d|%v|%s;", p.ID, p.Status, p.Updated.UnixMilli(), p.Title(), p.Meta.Starred, p.Meta.Archived, p.Meta.KennelID, p.Meta.Unfiled, p.Pane+"|"+p.Meta.LookRole+"|"+p.Meta.LookFur)
	}
	for _, t := range s.App.Things() {
		fmt.Fprintf(h, "%s|%v;", t.Key(), t.Running)
	}
	for _, k := range s.App.Kennels() {
		fmt.Fprintf(h, "k%d|%s|%s|%d|%s;", k.ID, k.Name, k.Icon, k.Floor, k.Smart)
	}
	for _, f := range s.App.Floors() {
		fmt.Fprintf(h, "f%d|%s|%s|%d|%s;", f.ID, f.Name, f.Icon, f.Sort, f.Space)
	}
	for _, sp := range s.App.Spaces() {
		fmt.Fprintf(h, "sp%d|%s|%s|%s;", sp.ID, sp.Name, sp.Icon, sp.Root)
	}
	for _, t := range s.App.Treats() {
		fmt.Fprintf(h, "t%d|%d|%s;", t.ID, t.Kennel, t.Name)
	}
	hidden, _ := s.App.Store.HiddenArtifacts()
	fmt.Fprintf(h, "ha%d;", len(hidden))
	styles := s.App.Styles()
	keys := make([]string, 0, len(styles))
	for k := range styles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b, _ := json.Marshal(styles[k])
		fmt.Fprintf(h, "s%s|%s;", k, b)
	}
	return h.Sum64()
}

func (s *Server) Version() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(static, "static")
	files := http.FileServer(http.FS(sub))
	mux.Handle("GET /", files)
	mux.HandleFunc("GET /api/state", s.guard(s.handleState))
	mux.HandleFunc("GET /api/events", s.guard(s.handleEvents))
	mux.HandleFunc("GET /api/search", s.guard(s.handleSearch))
	mux.HandleFunc("GET /api/transcript/{id}", s.guard(s.handleTranscript))
	mux.HandleFunc("POST /api/ask", s.guard(s.handleAsk))
	mux.HandleFunc("POST /api/pup/{id}/{action}", s.guard(s.handlePupAction))
	mux.HandleFunc("POST /api/thing/{action}", s.guard(s.handleThingAction))
	mux.HandleFunc("GET /api/catalog", s.guard(s.handleCatalog))
	mux.HandleFunc("GET /api/treats", s.guard(s.handleTreats))
	mux.HandleFunc("POST /api/treats", s.guard(s.handleAddTreat))
	mux.HandleFunc("POST /api/treats/{id}/delete", s.guard(s.handleDeleteTreat))
	mux.HandleFunc("POST /api/treats/{id}/shelve", s.guard(s.handleShelveTreat))
	mux.HandleFunc("POST /api/artifacts/hide", s.guard(s.handleHideArtifact))
	mux.HandleFunc("POST /api/open", s.guard(s.handleOpenFile))
	mux.HandleFunc("POST /api/sort", s.guard(s.handleSort))
	mux.HandleFunc("POST /api/sort/apply", s.guard(s.handleSortApply))
	mux.HandleFunc("POST /api/drop", s.guard(s.handleDrop))
	mux.HandleFunc("POST /api/undo", s.guard(s.handleUndo))
	mux.HandleFunc("POST /api/kennels", s.guard(s.handleAddKennel))
	mux.HandleFunc("POST /api/kennels/{id}/{action}", s.guard(s.handleKennel))
	mux.HandleFunc("POST /api/floors", s.guard(s.handleAddFloor))
	mux.HandleFunc("POST /api/spaces", s.guard(s.handleAddSpace))
	mux.HandleFunc("POST /api/spaces/{id}/{action}", s.guard(s.handleSpace))
	mux.HandleFunc("POST /api/floors/suggest", s.guard(s.handleSuggestFloors))
	mux.HandleFunc("POST /api/floors/organize", s.guard(s.handleOrganizeFloors))
	mux.HandleFunc("POST /api/floors/order", s.guard(s.handleFloorOrder))
	mux.HandleFunc("POST /api/floors/{id}/{action}", s.guard(s.handleFloor))
	mux.HandleFunc("GET /api/library", s.guard(s.handleLibrary))
	mux.HandleFunc("POST /api/themes/run", s.guard(s.handleThemes))
	mux.HandleFunc("GET /api/jobs", s.guard(s.handleJobs))
	mux.HandleFunc("GET /api/artifacts", s.guard(s.handleArtifacts))
	mux.HandleFunc("POST /api/room/{action}", s.guard(s.handleRoom))
	mux.HandleFunc("POST /api/appearance", s.guard(s.handleAppearance))
	mux.HandleFunc("POST /api/prefs", s.guard(s.handlePrefs))
	mux.HandleFunc("POST /api/term/{id}/{action}", s.guard(s.handleTerm))
	mux.HandleFunc("GET /api/term-theme", s.guard(s.handleTermTheme))
	mux.HandleFunc("GET /api/term/{id}/stream", s.guard(s.handleTermStream))
	return s.hostCheck(mux)
}

// hostCheck rejects requests whose Host isn't loopback, which blocks DNS-rebinding pages.
func (s *Server) hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "kennel only answers on localhost", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// guard requires the per-run token, so other sites can't drive the API from a browser.
func (s *Server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-Kennel-Token")
		if tok == "" {
			tok = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.Token)) != 1 {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error, code int) {
	if code >= 400 {
		log.Printf("kennel: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// ---- state ----

type PupJSON struct {
	ID          string   `json:"id"`
	Agent       string   `json:"agent"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Repo        string   `json:"repo"`
	Branch      string   `json:"branch"`
	Snippet     string   `json:"snippet"`
	FirstPrompt string   `json:"firstPrompt"`
	Recap       string   `json:"recap"`
	Updated     int64    `json:"updated"`
	Started     int64    `json:"started"`
	Since       int64    `json:"since"`
	Fur         string   `json:"fur"`
	Starred     bool     `json:"starred"`
	Archived    bool     `json:"archived"`
	Remote      bool     `json:"remote"`
	Crons       int      `json:"crons"`
	PRs         []string `json:"prs"`
	Artifacts   int      `json:"artifacts"`
	Kennel      string   `json:"kennel"`
	KennelID    int64    `json:"kennelId"`
	Theme       string   `json:"theme"`
	Unread      bool     `json:"unread"`
	Messages    int      `json:"messages"`
	Cost        float64  `json:"cost"`
	Pane        string   `json:"pane"`
	Cwd         string   `json:"cwd"`
	Project     string   `json:"project"`
	// Term is set while the pup runs in kennel's own terminal panel.
	Term bool   `json:"term"`
	Role string `json:"role"`
	// LookRole and LookFur are what the user picked ("" = automatic).
	LookRole string `json:"lookRole"`
	LookFur  string `json:"lookFur"`
	Note     string `json:"note"`
}

type ThingJSON struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
	Kind    string `json:"kind"`
	Icon    string `json:"icon"`
	Running bool   `json:"running"`
	Saved   bool   `json:"saved"`
	Pane    string `json:"pane"`
}

type RoomJSON struct {
	Key     string      `json:"key"`
	Kennel  int64       `json:"kennel"`
	Smart   string      `json:"smart"`
	Style   store.Style `json:"style"`
	MinCols int         `json:"minCols"`
	MinRows int         `json:"minRows"`
	Title   string      `json:"title"`
	Icon    string      `json:"icon"`
	Color   string      `json:"color"`
	Look    pack.Look   `json:"look"`
	Blurb   string      `json:"blurb"`
	Pups    []string    `json:"pups"`
	Things  []string    `json:"things"`
	Older   int         `json:"older"`
}

type StateJSON struct {
	Version int64 `json:"version"`
	Now     int64 `json:"now"`
	// Space is the project whose map this is; SpaceRoot its folder, if linked.
	Space     string             `json:"space"`
	SpaceRoot string             `json:"spaceRoot"`
	Spaces    []SpaceJSON        `json:"spaces"`
	Floor     int64              `json:"floor"`
	Floors    []FloorJSON        `json:"floors"`
	Rooms     []RoomJSON         `json:"rooms"`
	Pups      map[string]PupJSON `json:"pups"`
	// Tray lists recent and live pups from every floor, newest first, for the side panel.
	Tray []string `json:"tray"`
	// Handoffs are the last day's passes between sessions, drawn as lines.
	Handoffs []HandoffJSON `json:"handoffs"`
	// Where maps each pup to the floor it stands on in this project, for jumping to it.
	Where    map[string]int64     `json:"where"`
	Projects []ProjectJSON        `json:"projects"`
	Things   map[string]ThingJSON `json:"things"`
	Counts   map[string]int       `json:"counts"`
	Demo     bool                 `json:"demo"`
	// Indexing reports first-run progress ("12/230"); empty when idle.
	Indexing string `json:"indexing"`
	// Undo names the action ⌘Z would reverse; empty when there's nothing to undo.
	Undo    string       `json:"undo"`
	Kennels []KennelJSON `json:"kennels"`
	Themes  ThemeStatus  `json:"themes"`
	// Appearance is "light", "dark" or "" (follow the system), shared by every open page.
	Appearance string `json:"appearance"`
	// Prefs are the page's remembered filters (tray, library), as JSON strings.
	Prefs map[string]string `json:"prefs"`
}

type KennelJSON struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Floor int64  `json:"floor"`
	Smart string `json:"smart"`
}

type HandoffJSON struct {
	From string `json:"from"`
	To   string `json:"to"`
	At   int64  `json:"at"`
}

type SpaceJSON struct {
	Key     string `json:"key"`
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	Root    string `json:"root"`
	Floors  int    `json:"floors"`
	Kennels int    `json:"kennels"`
}

type FloorJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	// What's on the floor, for the navigator.
	Kennels int `json:"kennels"`
	Pups    int `json:"pups"`
	Waiting int `json:"waiting"`
}

type ProjectJSON struct {
	Root  string `json:"root"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

var statusNames = map[claude.Status]string{claude.Offline: "offline", claude.Idle: "idle", claude.Done: "done", claude.Working: "working", claude.Waiting: "waiting"}

func pupJSON(p *pack.Pup) PupJSON {
	j := PupJSON{ID: p.ID, Agent: p.AgentKind(), Title: p.Title(), Status: statusNames[p.Status], Repo: p.Repo(), Branch: p.Branch(), Snippet: p.Snippet(),
		FirstPrompt: p.FirstPrompt, Recap: p.Recap, Updated: p.Updated.UnixMilli(), Started: p.Started.UnixMilli(), Fur: furColor(p.ID, p.Color),
		Starred: p.Meta.Starred, Archived: p.Meta.Archived, Remote: p.Remote != "", Crons: len(p.Crons), PRs: p.PRs, Artifacts: len(p.Artifacts),
		Unread: p.Unread(), Messages: p.NUser, Cost: p.Cost, Pane: p.Pane, Cwd: p.Cwd, Project: app.ProjectRoot(p.Cwd), Role: p.Role(), Note: p.Meta.Note}
	// A look the user picked wins over the automatic one.
	switch p.Meta.LookRole {
	case "":
	case "none":
		j.Role = ""
	default:
		j.Role = p.Meta.LookRole
	}
	if p.Meta.LookFur != "" {
		j.Fur = p.Meta.LookFur
	}
	j.LookRole, j.LookFur = p.Meta.LookRole, p.Meta.LookFur
	if !p.Live.Since.IsZero() {
		j.Since = p.Live.Since.UnixMilli()
	}
	if p.Kennel != nil {
		j.Kennel = p.Kennel.Name
		j.KennelID = p.Kennel.ID
	}
	j.Theme = p.Meta.Theme
	if j.PRs == nil {
		j.PRs = []string{}
	}
	return j
}

func thingJSON(t things.Thing) ThingJSON {
	return ThingJSON{Key: t.Key(), Name: t.Name, Command: t.Command, Cwd: t.Cwd, Kind: string(t.Kind), Icon: things.Icon[t.Kind], Running: t.Running, Saved: t.Saved, Pane: t.Pane}
}

var furs = []string{"#fab387", "#f9e2af", "#e6c9a8", "#c9a27e", "#f5c2e7", "#b4befe", "#94e2d5", "#eba0ac", "#d9d9d9", "#a6adc8"}

var named = map[string]string{"red": "#f38ba8", "blue": "#89b4fa", "green": "#a6e3a1", "yellow": "#f9e2af", "purple": "#cba6f7",
	"orange": "#fab387", "pink": "#f5c2e7", "cyan": "#89dceb", "teal": "#94e2d5", "white": "#cdd6f4"}

func furColor(id, c string) string {
	if v, ok := named[c]; ok {
		return v
	}
	h := fnv.New32a()
	h.Write([]byte(id))
	return furs[h.Sum32()%uint32(len(furs))]
}

// State is one floor of a space's map plus everything the side panel needs.
func (s *Server) State(space string, fl int64) StateJSON {
	st := StateJSON{Version: s.Version(), Now: s.App.Now().UnixMilli(), Pups: map[string]PupJSON{}, Things: map[string]ThingJSON{},
		Counts: map[string]int{}, Demo: s.App.Cfg.Demo, Floors: []FloorJSON{}, Tray: []string{}, Projects: []ProjectJSON{}, Rooms: []RoomJSON{},
		Prefs: map[string]string{}, Spaces: []SpaceJSON{}, Where: map[string]int64{}, Handoffs: []HandoffJSON{}}
	if hs, err := s.App.Store.Handoffs(s.App.Now().Add(-24 * time.Hour)); err == nil {
		for _, h := range hs {
			st.Handoffs = append(st.Handoffs, HandoffJSON{h.From, h.To, h.At.UnixMilli()})
		}
	}
	space, floors, _ := s.App.OpenSpace(space)
	st.Space = space
	if sp, ok := s.App.Space(space); ok {
		st.SpaceRoot = sp.Root
	}
	for _, sp := range s.App.Spaces() {
		sj := SpaceJSON{Key: sp.Key(), ID: sp.ID, Name: sp.Name, Icon: sp.Icon, Root: sp.Root, Floors: len(s.App.FloorsIn(sp.Key()))}
		for _, k := range s.App.Kennels() {
			if s.App.SpaceOf(k) == sp.Key() {
				sj.Kennels++
			}
		}
		st.Spaces = append(st.Spaces, sj)
	}
	for _, f := range floors {
		fj := FloorJSON{ID: f.ID, Name: f.Name, Icon: f.Icon}
		for _, r := range floor.Rooms(s.App, f.ID) {
			fj.Kennels++
			fj.Pups += len(r.Pups)
			for _, p := range r.Pups {
				if _, seen := st.Where[p.ID]; !seen {
					st.Where[p.ID] = f.ID
				}
				if p.Status == claude.Waiting {
					fj.Waiting++
				}
			}
		}
		st.Floors = append(st.Floors, fj)
		if f.ID == fl {
			st.Floor = fl
		}
	}
	if st.Floor == 0 && len(st.Floors) > 0 {
		st.Floor = st.Floors[0].ID
	}
	now := s.App.Now()
	for _, p := range s.App.Pups() {
		st.Counts[statusNames[p.Status]]++
		if !p.Meta.Archived && !p.Empty() && (p.Status.Live() || now.Sub(p.Updated) < floor.Recent) && s.App.InSpace(p, space) {
			st.Tray = append(st.Tray, p.ID)
			st.Pups[p.ID] = s.pupJSON(p)
		}
	}
	for _, pr := range s.App.Projects() {
		st.Projects = append(st.Projects, ProjectJSON{pr.Root, pr.Name, pr.Count})
	}
	st.Indexing, _ = s.indexing.Load().(string)
	st.Undo = s.App.UndoLabel()
	st.Themes, _ = s.themes.Load().(ThemeStatus)
	if st.Themes.Provider == "" {
		_, st.Themes.Provider = s.App.ThemeLLM()
	}
	st.Kennels = []KennelJSON{}
	for _, k := range s.App.Kennels() {
		if s.App.SpaceOf(k) == space {
			st.Kennels = append(st.Kennels, KennelJSON{k.ID, k.Name, k.Icon, s.App.FloorOf(k), k.Smart})
		}
	}
	for _, key := range prefKeys {
		if v := s.App.Store.Get("pref:" + key); v != "" {
			st.Prefs[key] = v
		}
	}
	for _, r := range floor.Rooms(s.App, st.Floor) {
		rj := RoomJSON{Key: r.Key, Kennel: r.KennelID, Smart: r.Smart, Older: r.Older, Style: r.Style, MinCols: r.MinCols, MinRows: r.MinRows, Title: r.Title, Icon: r.Icon, Color: r.Color, Look: r.Look, Blurb: r.Blurb, Pups: []string{}, Things: []string{}}
		for _, p := range r.Pups {
			rj.Pups = append(rj.Pups, p.ID)
			st.Pups[p.ID] = s.pupJSON(p)
		}
		for _, t := range r.Things {
			rj.Things = append(rj.Things, t.Key())
			st.Things[t.Key()] = thingJSON(t)
		}
		st.Rooms = append(st.Rooms, rj)
	}
	return st
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	// The page remembers which space it was in and which floor of each space.
	q := r.URL.Query()
	space := s.App.Store.Get("web_space")
	if q.Has("space") {
		space = q.Get("space")
		_ = s.App.Store.Set("web_space", space)
	}
	var fl int64
	fmt.Sscan(s.App.Store.Get("web_floor:"+space), &fl)
	if v := q.Get("floor"); v != "" {
		fmt.Sscan(v, &fl)
		_ = s.App.Store.Set("web_floor:"+space, fmt.Sprint(fl))
	}
	st := s.State(space, fl)
	st.Appearance = s.App.Store.Get("web_appearance")
	writeJSON(w, st)
}

func (s *Server) handleAppearance(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Appearance string `json:"appearance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Appearance != "" && body.Appearance != "light" && body.Appearance != "dark") {
		fail(w, errors.New(`appearance is "light", "dark" or ""`), http.StatusBadRequest)
		return
	}
	if err := s.App.Store.Set("web_appearance", body.Appearance); err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	s.touch()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, errors.New("streaming unsupported"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	runs := make(chan []byte, 8)
	s.subMu.Lock()
	if s.subs == nil {
		s.subs = map[chan []byte]bool{}
	}
	s.subs[runs] = true
	s.subMu.Unlock()
	defer func() {
		s.subMu.Lock()
		delete(s.subs, runs)
		s.subMu.Unlock()
	}()
	last := int64(-1)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		if v := s.Version(); v != last {
			last = v
			fmt.Fprintf(w, "event: change\ndata: %d\n\n", v)
			fl.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case b := <-runs:
			fmt.Fprintf(w, "event: run\ndata: %s\n\n", b)
			fl.Flush()
		case <-tick.C:
		}
	}
}

type hitJSON struct {
	Role    string `json:"role"`
	Snippet string `json:"snippet"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	res, err := s.App.Search(r.URL.Query().Get("q"), true)
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	type item struct {
		Pup  PupJSON   `json:"pup"`
		Hits []hitJSON `json:"hits"`
	}
	out := []item{}
	for i, x := range res {
		if i == 50 {
			break
		}
		it := item{Pup: pupJSON(x.Pup), Hits: []hitJSON{}}
		for _, h := range x.Hits {
			it.Hits = append(it.Hits, hitJSON{Role: string(h.Role), Snippet: h.Snippet})
		}
		out = append(out, it)
	}
	writeJSON(w, out)
}

func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	p := s.App.Pup(r.PathValue("id"))
	if p == nil {
		fail(w, errors.New("no such session"), http.StatusNotFound)
		return
	}
	chunks, err := s.App.Store.Chunks(p.ID)
	if err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	type chunk struct {
		Role string `json:"role"`
		TS   int64  `json:"ts"`
		Text string `json:"text"`
	}
	out := []chunk{}
	// ?last=N is the conversation tail the pup card shows: the last N prompts and replies.
	last, _ := strconv.Atoi(r.URL.Query().Get("last"))
	for _, c := range chunks {
		if last > 0 && c.Role != claude.RoleUser && c.Role != claude.RoleAssistant {
			continue
		}
		out = append(out, chunk{Role: string(c.Role), TS: c.TS.UnixMilli(), Text: c.Text})
	}
	if last > 0 && len(out) > last {
		out = out[len(out)-last:]
	}
	writeJSON(w, map[string]any{"pup": pupJSON(p), "chunks": out})
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q string `json:"q"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Q) == "" {
		fail(w, errors.New("missing question"), http.StatusBadRequest)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, errors.New("streaming unsupported"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	type src struct {
		N       int    `json:"n"`
		ID      string `json:"id"`
		Title   string `json:"title"`
		Repo    string `json:"repo"`
		When    int64  `json:"when"`
		Role    string `json:"role"`
		Excerpt string `json:"excerpt"`
	}
	s.App.Asker().Ask(r.Context(), req.Q, func(ev ask.Event) {
		out := map[string]any{"stage": ev.Stage, "terms": ev.Terms, "text": ev.Text, "done": ev.Done}
		if ev.Err != nil {
			out["error"] = ev.Err.Error()
		}
		var srcs []src
		for _, x := range ev.Sources {
			srcs = append(srcs, src{N: x.N, ID: x.SessionID, Title: x.Title, Repo: x.Repo, When: x.When.UnixMilli(), Role: string(x.Hit.Role), Excerpt: x.Hit.Text})
		}
		out["sources"] = srcs
		b, _ := json.Marshal(out)
		fmt.Fprintf(w, "data: %s\n\n", b)
		fl.Flush()
	})
}

func (s *Server) handlePupAction(w http.ResponseWriter, r *http.Request) {
	p := s.App.Pup(r.PathValue("id"))
	if p == nil {
		fail(w, errors.New("no such session"), http.StatusNotFound)
		return
	}
	var body struct {
		Title  string   `json:"title"`
		Role   string   `json:"role"`
		Fur    string   `json:"fur"`
		From   string   `json:"from"`
		Note   string   `json:"note"`
		Cwd    string   `json:"cwd"`
		With   []string `json:"with"`
		Agent  string   `json:"agent"`
		Treat  int64    `json:"treat"`
		Value  string   `json:"value"`
		Kennel int64    `json:"kennel"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var msg string
	var err error
	switch r.PathValue("action") {
	case "open", "fork", "remote":
		act := r.PathValue("action")
		t := floor.TargetFor(s.App, p, act == "fork", act == "remote")
		if err = s.App.CheckCwd(t.Cwd); err == nil {
			if opener.Resolve(s.App.Cfg.Opener) == opener.Inline {
				// A browser can't hand over a terminal; give the command instead.
				msg = t.ShellCommand()
			} else {
				msg, err = s.App.Opener().Open(t)
			}
		}
		_ = s.App.MarkSeen(p.ID)
	case "star":
		err = s.App.ToggleStar(p.ID)
	case "archive":
		err = s.App.ToggleArchive(p.ID)
	case "rename":
		title := strings.TrimSpace(body.Title)
		if err = s.App.Rename(p.ID, title); err != nil || p.AgentKind() != "claude" || title == "" {
			break
		}
		// A running session gets the same /rename it would get by hand, where kennel can type.
		rename := "/rename " + opener.SafeArg(title)
		switch t := s.Terms.Get(p.ID); {
		case t != nil:
			if err = t.Write([]byte(rename + "\r")); err == nil {
				msg = "✏️ renamed " + title + ", in the session too"
			}
		case !p.Status.Live():
			msg = "✏️ renamed " + title + "; claude --resume shows it too"
		case p.Pane != "":
			if err = opener.Prompt(s.App.Cfg.Runner, p.Pane, rename); err == nil {
				msg = "✏️ renamed " + title + ", in the session too"
			}
		default:
			msg = "✏️ renamed " + title + " in kennel; type " + rename + " in its terminal to rename the session too"
		}

	case "clone":
		if p.AgentKind() != "claude" {
			err = errors.New("clone works on Claude sessions; use Convert for other agents")
			break
		}
		var id string
		if id, err = s.App.Clone(p.ID, app.ExpandHome(strings.TrimSpace(body.Cwd)), strings.TrimSpace(body.Title)); err == nil {
			c := s.App.Pup(id)
			if c == nil {
				err = errors.New("cloned, but the copy didn't show up in the index")
				break
			}
			msg = "⧉ cloned as " + c.Title()
			if opener.Resolve(s.App.Cfg.Opener) != opener.Inline {
				var opened string
				if opened, err = s.App.Opener().Open(s.App.Target(c, false, false)); err == nil {
					msg += " · " + opened
				}
			}
		}
	case "look":
		err = s.App.SetLook(p.ID, body.Role, body.Fur)
	case "handoff":
		var reached bool
		var text string
		if msg, reached, text, err = s.passOn(p, s.App.Pup(body.From), body.Note); err == nil {
			s.touch()
			writeJSON(w, map[string]any{"ok": true, "message": msg, "reached": reached, "text": text, "term": s.Terms.Get(p.ID) != nil})
			return
		}
	case "move":
		err = s.App.MoveTo(p.ID, body.Kennel)
		msg = "🏠 filed " + p.Title()
	case "feed":
		var tr treats.Treat
		if body.Treat != 0 {
			for _, t := range s.App.Treats() {
				if t.ID == body.Treat {
					tr = t
				}
			}
		} else if strings.TrimSpace(body.Value) != "" {
			tr = treats.New(body.Title, app.ExpandHome(body.Value))
		}
		if tr.Value == "" {
			err = errors.New("nothing to feed")
			break
		}
		prompt := tr.Prompt("")
		var live bool
		if live, err = s.App.FeedLive(p, prompt); live {
			msg = "🦴 fed " + p.Title() + " " + treats.Icon[tr.Kind] + " " + tr.Name
			break
		}
		var files []string
		if tr.Kind == treats.File {
			files = []string{tr.Value}
		}
		t := s.App.FeedTarget(p, prompt, files)
		if err = s.App.CheckCwd(t.Cwd); err == nil {
			if opener.Resolve(s.App.Cfg.Opener) == opener.Inline && t.Agent != "cursor" {
				msg = t.ShellCommand()
			} else if msg, err = s.App.Opener().Open(t); err == nil {
				msg = "🦴 woke " + p.Title() + ": " + msg
			}
		}
	case "export":
		msg, err = s.App.Export(p)
		if err == nil {
			msg = "⬇ exported to " + msg
		}
	case "convert":
		var t opener.Target
		var path string
		if t, path, err = s.App.ConvertTarget(p.ID, body.Agent, ""); err == nil {
			if opener.Resolve(s.App.Cfg.Opener) == opener.Inline && t.Agent != "cursor" {
				msg = t.ShellCommand()
			} else if msg, err = s.App.Opener().Open(t); err == nil {
				msg = "↔ " + msg + " · handoff " + path
			}
		}
	case "merge", "distill":
		ids := []string{p.ID}
		for _, ref := range body.With {
			o := s.App.Find(ref)
			if o == nil {
				err = fmt.Errorf("no pup matches %q", ref)
				break
			}
			ids = append(ids, o.ID)
		}
		if err == nil {
			var t opener.Target
			if t, err = s.App.MergeTarget(ids, "", strings.TrimSpace(body.Title)); err == nil {
				if opener.Resolve(s.App.Cfg.Opener) == opener.Inline {
					msg = t.ShellCommand()
				} else {
					msg, err = s.App.Opener().Open(t)
				}
			}
		}
	default:
		fail(w, errors.New("unknown action"), http.StatusNotFound)
		return
	}
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]any{"ok": true, "message": msg, "pup": pupJSON(s.App.Pup(p.ID))})
}

func (s *Server) handleThingAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	t := s.App.Thing(body.Key)
	if t == nil {
		fail(w, errors.New("no such thing"), http.StatusNotFound)
		return
	}
	var msg string
	var err error
	switch r.PathValue("action") {
	case "start":
		msg, err = s.App.StartOrFocus(*t)
	case "stop":
		err = s.App.Stop(*t)
		msg = "stopped " + t.Name
	default:
		fail(w, errors.New("unknown action"), http.StatusNotFound)
		return
	}
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": msg})
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"shapes": blocks.Shapes, "floors": blocks.Floors, "walls": blocks.Walls,
		"decor": blocks.DecorItems, "kinds": blocks.Kinds, "templates": blocks.Templates, "smarts": floor.SmartKinds()})
}

// handleRoom edits a room's building blocks: style, template, reset, door (spawn).
func (s *Server) handleRoom(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key      string      `json:"key"`
		Style    store.Style `json:"style"`
		Template string      `json:"template"`
		Index    int         `json:"index"`
		Prompt   string      `json:"prompt"`
		Title    string      `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" {
		fail(w, errors.New("missing room key"), http.StatusBadRequest)
		return
	}
	var msg string
	var err error
	switch r.PathValue("action") {
	case "style":
		err = s.App.SaveStyle(body.Key, body.Style)
	case "template":
		for _, tp := range blocks.Templates {
			if tp.Key == body.Template {
				err = floor.ApplyTemplate(s.App, body.Key, tp)
				msg = tp.Icon + " " + tp.Name
			}
		}
		if msg == "" && err == nil {
			err = fmt.Errorf("unknown template %q", body.Template)
		}
	case "reset":
		err = s.App.ResetStyle(body.Key)
	case "new":
		room, ok := floor.FindRoom(s.App, body.Key)
		if !ok {
			err = errors.New("no such room")
			break
		}
		var t opener.Target
		if t, err = floor.NewPupIn(s.App, room, body.Title, body.Prompt); err == nil {
			if err = s.App.CheckCwd(t.Cwd); err == nil {
				if opener.Resolve(s.App.Cfg.Opener) == opener.Inline {
					msg = t.ShellCommand()
				} else if msg, err = s.App.Opener().Open(t); err == nil {
					msg = "🐣 " + msg
				}
			}
		}
	case "door":
		room, ok := floor.FindRoom(s.App, body.Key)
		doors := room.Style.OfKind(blocks.Door)
		if !ok || body.Index < 0 || body.Index >= len(doors) {
			err = errors.New("no such door")
			break
		}
		var t opener.Target
		if t, err = floor.Spawn(s.App, room, doors[body.Index]); err == nil {
			if p := strings.TrimSpace(body.Prompt); p != "" {
				t.Prompt = p
			}
			if opener.Resolve(s.App.Cfg.Opener) == opener.Inline {
				msg = t.ShellCommand()
			} else {
				msg, err = s.App.Opener().Open(t)
			}
		}
	default:
		fail(w, errors.New("unknown action"), http.StatusNotFound)
		return
	}
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]any{"ok": true, "message": msg})
}

func (s *Server) handleTreats(w http.ResponseWriter, r *http.Request) {
	ts := s.App.Treats()
	type tj struct {
		treats.Treat
		Icon string `json:"icon"`
	}
	out := []tj{}
	for _, t := range ts {
		out = append(out, tj{t, treats.Icon[t.Kind]})
	}
	writeJSON(w, out)
}

func (s *Server) handleAddTreat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Kennel int64  `json:"kennel"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	t, err := s.App.AddTreat(body.Name, body.Value, body.Kennel)
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, t)
}

func (s *Server) handleShelveTreat(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.PathValue("id"), &id)
	var body struct {
		Kennel int64 `json:"kennel"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.App.ShelveTreat(id, body.Kennel); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleHideArtifact(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		fail(w, errors.New("which artifact?"), http.StatusBadRequest)
		return
	}
	if err := s.App.HideArtifact(body.URL); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]bool{"ok": true})
}

// handleOpenFile opens a saved file with the Mac's default app; links open in the page.
func (s *Server) handleOpenFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	path := app.ExpandHome(strings.TrimSpace(body.Path))
	if !strings.HasPrefix(path, "/") {
		fail(w, errors.New("only files can be opened this way"), http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(path); err != nil {
		fail(w, fmt.Errorf("%s isn't there any more", path), http.StatusNotFound)
		return
	}
	if _, err := s.App.Cfg.Runner.Run("open", "--", path); err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteTreat(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.PathValue("id"), &id)
	if err := s.App.DeleteTreat(id); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]bool{"ok": true})
}

const maxDrop = 25 << 20

// handleDrop saves a file dropped from the desktop so an agent can read it by path.
func (s *Server) handleDrop(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDrop*2)
	var body struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, fmt.Errorf("drop too large or malformed: %w", err), http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil || len(data) > maxDrop {
		fail(w, errors.New("drop must be base64 and at most 25 MB"), http.StatusBadRequest)
		return
	}
	path, err := s.App.SaveDrop(body.Name, data)
	if err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"path": path})
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	label, err := s.App.Undo()
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]any{"ok": true, "message": "↶ undid " + label, "undo": s.App.UndoLabel()})
}

func (s *Server) handleAddKennel(w http.ResponseWriter, r *http.Request) {
	var k store.Kennel
	if err := json.NewDecoder(r.Body).Decode(&k); err != nil || strings.TrimSpace(k.Name) == "" {
		fail(w, errors.New("a kennel needs a name"), http.StatusBadRequest)
		return
	}
	k.ID = 0
	k.Name = strings.TrimSpace(k.Name)
	if k.Smart != "" {
		kind, ok := floor.SmartKindByKey(k.Smart)
		if !ok {
			fail(w, fmt.Errorf("unknown smart kennel %q", k.Smart), http.StatusBadRequest)
			return
		}
		if k.Icon == "" {
			k.Icon = kind.Icon
		}
	}
	// Names are unique; a second Firehouse on another floor becomes "Firehouse 2".
	base := k.Name
	for n := 2; s.kennelNamed(k.Name); n++ {
		k.Name = fmt.Sprintf("%s %d", base, n)
	}
	if err := s.App.SaveKennel(&k); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, k)
}

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	out := []PupJSON{}
	for _, p := range s.App.Pups() {
		if !p.Empty() {
			out = append(out, pupJSON(p))
		}
	}
	writeJSON(w, out)
}

// handleThemes starts categorizing in the background; progress shows up in /api/state.
func (s *Server) handleThemes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs  []string `json:"ids"`
		Redo bool     `json:"redo"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if cur, _ := s.themes.Load().(ThemeStatus); cur.Running {
		fail(w, errors.New("already categorizing"), http.StatusConflict)
		return
	}
	_, provider := s.App.ThemeLLM()
	s.themes.Store(ThemeStatus{Running: true, Provider: provider})
	s.touch()
	go func() {
		err := s.App.Categorize(context.Background(), body.IDs, body.Redo, func(p app.ThemeProgress) {
			s.themes.Store(ThemeStatus{Running: true, Done: p.Done, Total: p.Total, Provider: provider, Themes: p.Themes})
			s.touch()
		})
		final := ThemeStatus{Provider: provider}
		if cur, ok := s.themes.Load().(ThemeStatus); ok {
			final.Done, final.Total, final.Themes = cur.Done, cur.Total, cur.Themes
		}
		if err != nil {
			final.Error = err.Error()
		}
		s.themes.Store(final)
		s.touch()
	}()
	writeJSON(w, map[string]any{"ok": true, "provider": provider})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	type jobJSON struct {
		Session string `json:"session"`
		Title   string `json:"title"`
		State   string `json:"state"`
		Detail  string `json:"detail"`
		Running bool   `json:"running"`
		Updated int64  `json:"updated"`
	}
	type cronJSON struct {
		Session  string `json:"session"`
		Title    string `json:"title"`
		Expr     string `json:"expr"`
		Prompt   string `json:"prompt"`
		Fires    int    `json:"fires"`
		LastFire int64  `json:"lastFire"`
		Live     bool   `json:"live"`
	}
	out := struct {
		Jobs   []jobJSON   `json:"jobs"`
		Crons  []cronJSON  `json:"crons"`
		Things []ThingJSON `json:"things"`
	}{Jobs: []jobJSON{}, Crons: []cronJSON{}, Things: []ThingJSON{}}
	for _, j := range s.App.Jobs() {
		title := strings.TrimSpace(strings.ReplaceAll(j.Name, "⑂", ""))
		if p := s.App.Pup(j.SessionID); p != nil {
			title = p.Title()
		}
		detail := j.Detail
		if detail == "" {
			detail = j.Intent
		}
		out.Jobs = append(out.Jobs, jobJSON{j.SessionID, title, j.State, detail, j.Running(), j.UpdatedAt.UnixMilli()})
	}
	for _, p := range s.App.Pups() {
		for _, c := range p.Crons {
			out.Crons = append(out.Crons, cronJSON{p.ID, p.Title(), c.Expr, c.Prompt, c.Fires, c.LastFire.UnixMilli(), p.Status.Live()})
		}
	}
	for _, t := range s.App.Things() {
		out.Things = append(out.Things, thingJSON(t))
	}
	writeJSON(w, out)
}

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	type artJSON struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		At      int64  `json:"at"`
		Session string `json:"session"`
		Pup     string `json:"pup"`
	}
	out := []artJSON{}
	for _, a := range s.App.Artifacts() {
		out = append(out, artJSON{a.URL, a.Title, a.At.UnixMilli(), a.Pup.ID, a.Pup.Title()})
	}
	writeJSON(w, out)
}

func (s *Server) kennelNamed(name string) bool {
	for _, k := range s.App.Kennels() {
		if strings.EqualFold(k.Name, name) {
			return true
		}
	}
	return false
}

// handleKennel renames, deletes or moves a kennel to another floor.
func (s *Server) handleKennel(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.PathValue("id"), &id)
	var k *store.Kennel
	for _, x := range s.App.Kennels() {
		if x.ID == id {
			x := x
			k = &x
		}
	}
	if k == nil {
		fail(w, fmt.Errorf("no kennel %d", id), http.StatusNotFound)
		return
	}
	var body struct {
		Name  string `json:"name"`
		Icon  string `json:"icon"`
		Floor int64  `json:"floor"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var err error
	switch r.PathValue("action") {
	case "rename":
		name := strings.TrimSpace(body.Name)
		if name == "" {
			err = errors.New("a kennel needs a name")
			break
		}
		if !strings.EqualFold(name, k.Name) && s.kennelNamed(name) {
			err = fmt.Errorf("there's already a kennel called %s", name)
			break
		}
		k.Name = name
		if body.Icon != "" {
			k.Icon = body.Icon
		}
		err = s.App.SaveKennel(k)
	case "delete":
		err = s.App.DeleteKennel(id)
	case "floor":
		err = s.App.MoveKennelToFloor(id, body.Floor)
	default:
		fail(w, errors.New("unknown action"), http.StatusNotFound)
		return
	}
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAddFloor(w http.ResponseWriter, r *http.Request) {
	var f store.Floor
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	f.ID = 0
	f.Name = strings.TrimSpace(f.Name)
	if err := s.App.SaveFloor(&f); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, f)
}

// handleFloor renames or removes a floor; a removed floor's kennels move to another one.
func (s *Server) handleFloor(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.PathValue("id"), &id)
	var body struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var err error
	switch r.PathValue("action") {
	case "rename":
		err = errors.New("no such floor")
		for _, f := range s.App.Floors() {
			if f.ID == id {
				f.Name = strings.TrimSpace(body.Name)
				if body.Icon != "" {
					f.Icon = body.Icon
				}
				err = s.App.SaveFloor(&f)
			}
		}
	case "delete":
		err = s.App.DeleteFloor(id)
	default:
		fail(w, errors.New("unknown action"), http.StatusNotFound)
		return
	}
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]any{"ok": true})
}

// handleSort suggests a kennel for each session; nothing is filed until apply.
func (s *Server) handleSort(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs   []string `json:"ids"`
		Space string   `json:"space"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	out, err := s.App.SuggestKennels(r.Context(), body.IDs, body.Space)
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, out)
}

func (s *Server) handleSortApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Moves []app.Suggestion `json:"moves"`
		Floor int64            `json:"floor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	n, err := s.App.FileAll(body.Moves, body.Floor)
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]any{"ok": true, "message": fmt.Sprintf("🗂 filed %d sessions", n)})
}

// prefKeys are the page filters kennel remembers between sessions and launches.
var prefKeys = []string{"tray", "library", "layout"}

func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !slices.Contains(prefKeys, body.Key) || len(body.Value) > 4096 {
		fail(w, errors.New("unknown or oversized preference"), http.StatusBadRequest)
		return
	}
	if err := s.App.Store.Set("pref:"+body.Key, body.Value); err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) pupJSON(p *pack.Pup) PupJSON {
	j := pupJSON(p)
	j.Term = s.Terms != nil && s.Terms.Get(p.ID) != nil
	return j
}

// handleTerm starts, feeds, resizes or stops a session's terminal in the panel.
func (s *Server) handleTerm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
		Data string `json:"data"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	t := s.Terms.Get(id)
	switch r.PathValue("action") {
	case "start":
		if t != nil {
			_ = t.Resize(body.Cols, body.Rows)
			break
		}
		p := s.App.Pup(id)
		if p == nil {
			fail(w, errors.New("no such session"), http.StatusNotFound)
			return
		}
		if err := s.startTerm(p, "", body.Cols, body.Rows); err != nil {
			fail(w, err, http.StatusBadRequest)
			return
		}
		s.touch()
	case "input":
		if t == nil {
			fail(w, errors.New("not running here"), http.StatusConflict)
			return
		}
		if err := t.Write([]byte(body.Data)); err != nil {
			fail(w, err, http.StatusConflict)
			return
		}
	case "resize":
		if t != nil {
			_ = t.Resize(body.Cols, body.Rows)
		}
	case "stop":
		if t != nil {
			t.Stop()
		}
		s.touch()
	default:
		fail(w, errors.New("unknown action"), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleTermStream sends a terminal's scrollback, then its output, as base64 events.
func (s *Server) handleTermStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	t := s.Terms.Get(r.PathValue("id"))
	if !ok || t == nil {
		fail(w, errors.New("not running here"), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	back, live, detach := t.Attach()
	defer detach()
	send := func(b []byte) {
		fmt.Fprintf(w, "data: %s\n\n", base64.StdEncoding.EncodeToString(b))
		fl.Flush()
	}
	if len(back) > 0 {
		send(back)
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case b, open := <-live:
			if !open {
				fmt.Fprint(w, "event: exit\ndata: {}\n\n")
				fl.Flush()
				s.touch()
				return
			}
			send(b)
		}
	}
}

func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

func (s *Server) handleSuggestFloors(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Space string `json:"space"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	plan, err := s.App.SuggestFloors(r.Context(), body.Space)
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, plan)
}

func (s *Server) handleOrganizeFloors(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Space string          `json:"space"`
		Plan  []app.FloorPlan `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	n, err := s.App.OrganizeFloors(body.Space, body.Plan)
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]any{"ok": true, "message": fmt.Sprintf("🏢 moved %d kennels onto their floors", n)})
}

func (s *Server) handleFloorOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Space string  `json:"space"`
		IDs   []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	if err := s.App.ReorderFloors(body.Space, body.IDs); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleAddSpace(w http.ResponseWriter, r *http.Request) {
	var sp struct {
		store.Space
		// Create makes the folder when it doesn't exist (for a brand-new project).
		Create bool
	}
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	sp.ID = 0
	// A project is a folder: named after it unless told otherwise, created if missing.
	sp.Root = app.ExpandHome(strings.TrimSpace(sp.Root))
	if sp.Root == "" {
		fail(w, errors.New("a project needs a folder"), http.StatusBadRequest)
		return
	}
	if sp.Create {
		if err := os.MkdirAll(sp.Root, 0o755); err != nil {
			fail(w, err, http.StatusBadRequest)
			return
		}
	}
	sp.Name = strings.TrimSpace(sp.Name)
	if sp.Name == "" {
		sp.Name = filepath.Base(sp.Root)
	}
	if sp.Icon == "" {
		sp.Icon = "📁"
	}
	if err := s.App.SaveSpace(&sp.Space); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]any{"key": sp.Key(), "name": sp.Name})
}

// handleSpace edits (name, icon, folder) or removes a project.
func (s *Server) handleSpace(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.PathValue("id"), &id)
	sp, ok := s.App.Space(fmt.Sprintf("space:%d", id))
	if !ok {
		fail(w, errors.New("no such project"), http.StatusNotFound)
		return
	}
	var body struct {
		Name *string `json:"name"`
		Icon *string `json:"icon"`
		Root *string `json:"root"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var err error
	switch r.PathValue("action") {
	case "edit":
		if body.Name != nil {
			sp.Name = strings.TrimSpace(*body.Name)
		}
		if body.Icon != nil {
			sp.Icon = *body.Icon
		}
		if body.Root != nil {
			sp.Root = app.ExpandHome(strings.TrimSpace(*body.Root))
		}
		err = s.App.SaveSpace(&sp)
	case "delete":
		err = s.App.DeleteSpace(id)
	default:
		fail(w, errors.New("unknown action"), http.StatusNotFound)
		return
	}
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	s.touch()
	writeJSON(w, map[string]bool{"ok": true})
}

// startTerm resumes a session in the panel's terminal, with an optional first prompt.
// A session live in another terminal may run here too: a second resume of the same
// conversation is what you asked for when you press Continue here.
func (s *Server) startTerm(p *pack.Pup, prompt string, cols, rows int) error {
	tg := floor.TargetFor(s.App, p, false, false)
	tg.Prompt = prompt
	if err := s.App.CheckCwd(tg.Cwd); err != nil {
		return err
	}
	bin, args := tg.Command()
	line := opener.CommandLine(bin, args)
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	cmdArgs := []string{"-l", "-c", line}
	// Demo and tests never start a real agent: the terminal says what it would run and echoes.
	if s.App.Cfg.Demo || opener.Resolve(s.App.Cfg.Opener) == opener.Print {
		shell, cmdArgs = "/bin/sh", []string{"-c", "printf '%s\\n' " + shellQuote("would run: "+line) + "; exec cat"}
	}
	dir := tg.Cwd
	if _, err := os.Stat(dir); err != nil {
		dir, _ = os.UserHomeDir() // demo sessions point at folders that don't exist
	}
	_, err := s.Terms.Start(p.ID, dir, shell, cmdArgs, nil, cols, rows)
	return err
}

// passOn passes one session's latest reply (and your note) to another, wherever
// kennel can reach it. reached is false when the text has to be pasted by hand.
func (s *Server) passOn(to, from *pack.Pup, note string) (msg string, reached bool, text string, err error) {
	if from == nil || from.ID == to.ID {
		return "", false, "", errors.New("pick another session to pass from")
	}
	text = opener.SafeArg(s.App.HandoffText(from, note))
	switch t := s.Terms.Get(to.ID); {
	case t != nil:
		// Bracketed paste keeps it one message.
		err = t.Write([]byte("\x1b[200~" + text + "\x1b[201~\r"))
		msg, reached = fmt.Sprintf("📨 %s → %s, in its terminal", from.Title(), to.Title()), true
	case to.Status.Live() && to.Pane != "":
		err = opener.Prompt(s.App.Cfg.Runner, to.Pane, text)
		msg, reached = fmt.Sprintf("📨 %s → %s, in its pane", from.Title(), to.Title()), true
	case !to.Status.Live() && to.AgentKind() == "claude":
		err = s.startTerm(to, text, 100, 30)
		msg, reached = fmt.Sprintf("📨 %s woke up with what %s found", to.Title(), from.Title()), true
	default:
		msg = fmt.Sprintf("📋 %s runs where kennel can't type: the message is on your clipboard", to.Title())
	}
	if err == nil {
		_ = s.App.RecordHandoff(from.ID, to.ID)
	}
	return msg, reached, text, err
}

// handleTermTheme is the user's Ghostty look, so the panel terminal matches Ghostty;
// demo data gets Ghostty's defaults rather than whatever this machine has.
func (s *Server) handleTermTheme(w http.ResponseWriter, r *http.Request) {
	if s.App.Cfg.Demo {
		writeJSON(w, ghostty.Defaults())
		return
	}
	home, _ := os.UserHomeDir()
	writeJSON(w, ghostty.Load(home))
}
