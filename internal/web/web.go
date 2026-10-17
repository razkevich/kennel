// Package web serves kennel's Gather-style map: a pixel-art office where your
// sessions wander around, backed by a small JSON + SSE API on localhost.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/ask"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/floor"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/things"
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
	s := &Server{App: a, Token: token, Herdr: !a.Cfg.Demo}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go s.pump(ctx)
	url := fmt.Sprintf("http://%s/?t=%s", ln.Addr().String(), s.Token)
	fmt.Println("▼•ᴥ•▼ kennel map at", url)
	if open {
		openBrowser(url)
	}
	go func() {
		<-ctx.Done()
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
		s.App.RefreshLive(s.Herdr && n%2 == 0)
		if n%3 == 0 {
			_, _ = s.App.Sync(ctx, nil)
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
	for _, p := range s.App.Pups() {
		fmt.Fprintf(h, "%s|%d|%d|%s|%v|%v|%d|%s;", p.ID, p.Status, p.Updated.UnixMilli(), p.Title(), p.Meta.Starred, p.Meta.Archived, p.Meta.KennelID, p.Pane)
	}
	for _, t := range s.App.Things() {
		fmt.Fprintf(h, "%s|%v;", t.Key(), t.Running)
	}
	for _, k := range s.App.Kennels() {
		fmt.Fprintf(h, "k%d|%s;", k.ID, k.Name)
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// ---- state ----

type PupJSON struct {
	ID          string   `json:"id"`
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
	Unread      bool     `json:"unread"`
	Messages    int      `json:"messages"`
	Cost        float64  `json:"cost"`
	Pane        string   `json:"pane"`
	Cwd         string   `json:"cwd"`
	Note        string   `json:"note"`
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
	Key    string    `json:"key"`
	Title  string    `json:"title"`
	Icon   string    `json:"icon"`
	Color  string    `json:"color"`
	Look   pack.Look `json:"look"`
	Blurb  string    `json:"blurb"`
	Pups   []string  `json:"pups"`
	Things []string  `json:"things"`
}

type StateJSON struct {
	Version int64                `json:"version"`
	Now     int64                `json:"now"`
	Floor   int                  `json:"floor"`
	Floors  []string             `json:"floors"`
	Rooms   []RoomJSON           `json:"rooms"`
	Pups    map[string]PupJSON   `json:"pups"`
	Things  map[string]ThingJSON `json:"things"`
	Counts  map[string]int       `json:"counts"`
	Demo    bool                 `json:"demo"`
}

var statusNames = map[claude.Status]string{claude.Offline: "offline", claude.Idle: "idle", claude.Done: "done", claude.Working: "working", claude.Waiting: "waiting"}

func pupJSON(p *pack.Pup) PupJSON {
	j := PupJSON{ID: p.ID, Title: p.Title(), Status: statusNames[p.Status], Repo: p.Repo(), Branch: p.Branch(), Snippet: p.Snippet(),
		FirstPrompt: p.FirstPrompt, Recap: p.Recap, Updated: p.Updated.UnixMilli(), Started: p.Started.UnixMilli(), Fur: furColor(p.ID, p.Color),
		Starred: p.Meta.Starred, Archived: p.Meta.Archived, Remote: p.Remote != "", Crons: len(p.Crons), PRs: p.PRs, Artifacts: len(p.Artifacts),
		Unread: p.Unread(), Messages: p.NUser, Cost: p.Cost, Pane: p.Pane, Cwd: p.Cwd, Note: p.Meta.Note}
	if !p.Live.Since.IsZero() {
		j.Since = p.Live.Since.UnixMilli()
	}
	if p.Kennel != nil {
		j.Kennel = p.Kennel.Name
	}
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

func (s *Server) State(fl int) StateJSON {
	if fl < 0 || fl >= len(floor.Names) {
		fl = 0
	}
	st := StateJSON{Version: s.Version(), Now: s.App.Now().UnixMilli(), Floor: fl, Floors: floor.Names,
		Pups: map[string]PupJSON{}, Things: map[string]ThingJSON{}, Counts: map[string]int{}, Demo: s.App.Cfg.Demo}
	for _, p := range s.App.Pups() {
		st.Counts[statusNames[p.Status]]++
	}
	for _, r := range floor.Rooms(s.App, fl) {
		rj := RoomJSON{Key: r.Key, Title: r.Title, Icon: r.Icon, Color: r.Color, Look: r.Look, Blurb: r.Blurb, Pups: []string{}, Things: []string{}}
		for _, p := range r.Pups {
			rj.Pups = append(rj.Pups, p.ID)
			st.Pups[p.ID] = pupJSON(p)
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
	fl := 0
	fmt.Sscan(r.URL.Query().Get("floor"), &fl)
	writeJSON(w, s.State(fl))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, errors.New("streaming unsupported"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
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
	_, chunks, err := claude.ParseFile(p.Path)
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
	for _, c := range chunks {
		out = append(out, chunk{Role: string(c.Role), TS: c.TS.UnixMilli(), Text: c.Text})
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
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var msg string
	var err error
	switch r.PathValue("action") {
	case "open", "fork", "remote":
		act := r.PathValue("action")
		t := s.App.Target(p, act == "fork", act == "remote")
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
		err = s.App.Rename(p.ID, strings.TrimSpace(body.Title))
	case "reply":
		err = s.App.Prompt(p, body.Text)
		msg = "sent"
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
