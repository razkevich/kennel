package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/ask"
	"github.com/razkevich/kennel/internal/demo"
	"github.com/razkevich/kennel/internal/digest"
	"github.com/razkevich/kennel/internal/indexer"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/tui"
	"github.com/razkevich/kennel/internal/web"
)

var version = "dev"

const usage = `▼•ᴥ•▼ kennel — find, organize and open your Claude Code sessions

usage:
  kennel                      open the TUI
  kennel map [-port N]        open the Gather-style web map in your browser
  kennel search <query>       search names and transcripts  (-json)
  kennel ask <question>       answer from past sessions, with sources
  kennel ls [query]           list sessions                 (-json)
  kennel digest [-since 24h]  what happened: touched, shipped, waiting, under the rug
  kennel open <id|name>       resume a session              (-fork, -remote, -opener herdr|ghostty|inline|print)
  kennel clone <ref> [-to dir] copy a session (optionally into another project) and resume it
  kennel merge <ref> <ref>... start a new pup seeded with a brief of several sessions (-name, -cwd)
  kennel distill <ref>        start a fresh pup from a brief of one long session
  kennel index                (re)index transcripts now     (-rebuild)
  kennel demo [dir]           write synthetic sessions to dir and print env to use them
  kennel version

global flags:
  -demo                       run against built-in synthetic sessions (never touches yours)

env:
  KENNEL_CLAUDE_DIR           Claude config dir to read (default ~/.claude)
  KENNEL_HOME                 kennel's own data dir (default ~/.kennel)
  KENNEL_OPENER               auto | herdr | ghostty | inline | print
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kennel:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	demoMode := false
	var rest []string
	for _, a := range args {
		if a == "-demo" || a == "--demo" {
			demoMode = true
			continue
		}
		rest = append(rest, a)
	}
	args = rest
	cmd := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "version":
		fmt.Println("kennel", version)
		return nil
	case "demo":
		dir := filepath.Join(os.TempDir(), "kennel-demo")
		if len(args) > 0 {
			dir = args[0]
		}
		if err := writeDemo(dir); err != nil {
			return err
		}
		fmt.Printf("export KENNEL_CLAUDE_DIR=%s/claude KENNEL_HOME=%s/home KENNEL_OPENER=print\n", dir, dir)
		return nil
	}

	cfg := app.DefaultConfig()
	if demoMode {
		dir, err := os.MkdirTemp("", "kennel-demo-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		if err := writeDemo(dir); err != nil {
			return err
		}
		cfg.ClaudeDir = filepath.Join(dir, "claude")
		cfg.Home = filepath.Join(dir, "home")
		if os.Getenv("KENNEL_OPENER") == "" {
			cfg.Opener = opener.Print
		}
	}

	switch cmd {
	case "":
		return runTUI(cfg, demoMode)
	case "map":
		return runMap(cfg, args)
	case "index":
		return runIndex(cfg, args)
	case "search", "ls":
		return runSearch(cfg, cmd, args)
	case "ask":
		return runAsk(cfg, args)
	case "open":
		return runOpen(cfg, args)
	case "digest":
		return runDigest(cfg, args)
	case "clone":
		return runClone(cfg, args)
	case "merge", "distill":
		return runMerge(cfg, args)
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
}

func writeDemo(dir string) error {
	return demo.Generate(filepath.Join(dir, "claude"), time.Now())
}

func openApp(cfg app.Config, sync bool) (*app.App, error) {
	a, err := app.New(cfg)
	if err != nil {
		return nil, err
	}
	if sync {
		if _, err := a.Sync(context.Background(), nil); err != nil {
			a.Close()
			return nil, err
		}
	}
	return a, nil
}

func runTUI(cfg app.Config, demoMode bool) error {
	a, err := openApp(cfg, false)
	if err != nil {
		return err
	}
	defer a.Close()
	zone.NewGlobal()
	m := tui.New(a, tui.Options{NoHerdr: demoMode})
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

func runIndex(cfg app.Config, args []string) error {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	rebuild := fs.Bool("rebuild", false, "drop the index and parse everything again")
	fs.Parse(args)
	if *rebuild {
		if err := os.Remove(filepath.Join(cfg.Home, "kennel.db")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	a, err := openApp(cfg, false)
	if err != nil {
		return err
	}
	defer a.Close()
	start := time.Now()
	p, err := a.Sync(context.Background(), func(p indexer.Progress) {
		fmt.Fprintf(os.Stderr, "\r🦴 %d/%d", p.Done, p.Total)
	})
	fmt.Fprintln(os.Stderr)
	sessions, chunks := a.Store.Count()
	fmt.Printf("indexed %d changed files in %s · %d sessions · %d searchable chunks\n", p.Changed, time.Since(start).Round(time.Millisecond), sessions, chunks)
	return err
}

func runSearch(cfg app.Config, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	limit := fs.Int("n", 20, "max results")
	fs.Parse(args)
	a, err := openApp(cfg, true)
	if err != nil {
		return err
	}
	defer a.Close()
	res, err := a.Search(strings.Join(fs.Args(), " "), cmd == "search")
	if err != nil {
		return err
	}
	if len(res) > *limit {
		res = res[:*limit]
	}
	type out struct {
		ID      string   `json:"id"`
		Title   string   `json:"title"`
		Status  string   `json:"status"`
		Repo    string   `json:"repo"`
		Branch  string   `json:"branch,omitempty"`
		Updated string   `json:"updated"`
		Cwd     string   `json:"cwd"`
		Matches []string `json:"matches,omitempty"`
		Resume  string   `json:"resume"`
	}
	var rows []out
	for _, r := range res {
		p := r.Pup
		o := out{ID: p.ID, Title: p.Title(), Status: p.Status.String(), Repo: p.Repo(), Branch: p.Branch(),
			Updated: p.Updated.Format(time.RFC3339), Cwd: p.Cwd, Resume: a.Target(p, false, false).ShellCommand()}
		for _, h := range r.Hits {
			o.Matches = append(o.Matches, strings.NewReplacer("\x01", "", "\x02", "").Replace(h.Snippet))
		}
		rows = append(rows, o)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	for _, o := range rows {
		fmt.Printf("%s  %-28s  %-9s  %-12s  %s\n", o.ID[:8], trunc(o.Title, 28), o.Status, trunc(o.Repo, 12), o.Updated[:10])
		for _, mt := range o.Matches {
			fmt.Printf("          › %s\n", trunc(mt, 100))
		}
	}
	if len(rows) == 0 {
		fmt.Println("no matches")
	}
	return nil
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func runAsk(cfg app.Config, args []string) error {
	q := strings.Join(args, " ")
	if q == "" {
		return errors.New("usage: kennel ask <question>")
	}
	a, err := openApp(cfg, true)
	if err != nil {
		return err
	}
	defer a.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	printed := 0
	var final ask.Event
	a.Asker().Ask(ctx, q, func(ev ask.Event) {
		if ev.Stage != "" && ev.Text == "" && !ev.Done {
			fmt.Fprintf(os.Stderr, "▼•ᴥ•▼ %s…\n", ev.Stage)
		}
		if len(ev.Text) > printed {
			fmt.Print(ev.Text[printed:])
			printed = len(ev.Text)
		}
		if ev.Done {
			final = ev
		}
	})
	fmt.Println()
	if final.Err != nil {
		return final.Err
	}
	if len(final.Sources) > 0 {
		fmt.Println("\nsources:")
		for _, s := range final.Sources {
			fmt.Printf("  [%d] %s (%s, %s)  claude --resume %s\n", s.N, s.Title, s.Repo, s.When.Format("2006-01-02"), s.SessionID)
		}
	}
	return nil
}

func runOpen(cfg app.Config, args []string) error {
	fs := flag.NewFlagSet("open", flag.ExitOnError)
	fork := fs.Bool("fork", false, "fork into a new session")
	remote := fs.Bool("remote", false, "enable remote control")
	kind := fs.String("opener", "", "herdr | ghostty | inline | print")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("usage: kennel open <id|name>")
	}
	if *kind != "" {
		cfg.Opener = opener.Kind(*kind)
	}
	a, err := openApp(cfg, true)
	if err != nil {
		return err
	}
	defer a.Close()
	p := a.Find(strings.Join(fs.Args(), " "))
	if p == nil {
		return fmt.Errorf("no session matches %q", strings.Join(fs.Args(), " "))
	}
	t := a.Target(p, *fork, *remote)
	if opener.Resolve(cfg.Opener) == opener.Inline {
		c := opener.InlineCmd(t)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	}
	msg, err := a.Opener().Open(t)
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

func runMap(cfg app.Config, args []string) error {
	fs := flag.NewFlagSet("map", flag.ExitOnError)
	port := fs.Int("port", 7317, "port to listen on (localhost only)")
	noOpen := fs.Bool("no-open", false, "don't open the browser")
	token := fs.String("token", "", "fixed API token (default: random per run)")
	fs.Parse(args)
	a, err := openApp(cfg, true)
	if err != nil {
		return err
	}
	defer a.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return web.Serve(ctx, a, fmt.Sprintf("127.0.0.1:%d", *port), !*noOpen, *token)
}

func runDigest(cfg app.Config, args []string) error {
	fs := flag.NewFlagSet("digest", flag.ExitOnError)
	since := fs.Duration("since", 24*time.Hour, "window to summarize")
	fs.Parse(args)
	a, err := openApp(cfg, true)
	if err != nil {
		return err
	}
	defer a.Close()
	d := digest.Build(a.Pups(), a.Now(), *since)
	window := strings.TrimSuffix(strings.TrimSuffix(since.String(), "0s"), "0m")
	if *since%(24*time.Hour) == 0 {
		window = fmt.Sprintf("%d day(s)", *since/(24*time.Hour))
	}
	fmt.Printf("▼•ᴥ•▼ kennel digest — last %s\n\n", window)
	for _, l := range d.Lines() {
		fmt.Println("  " + l)
	}
	return nil
}

func runClone(cfg app.Config, args []string) error {
	fs := flag.NewFlagSet("clone", flag.ExitOnError)
	to := fs.String("to", "", "project folder for the copy (default: same project)")
	name := fs.String("name", "", "title for the copy")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("usage: kennel clone <id|name> [-to dir]")
	}
	a, err := openApp(cfg, true)
	if err != nil {
		return err
	}
	defer a.Close()
	p := a.Find(strings.Join(fs.Args(), " "))
	if p == nil {
		return fmt.Errorf("no session matches %q", strings.Join(fs.Args(), " "))
	}
	id, err := a.Clone(p.ID, app.ExpandHome(*to), *name)
	if err != nil {
		return err
	}
	c := a.Pup(id)
	fmt.Printf("⧉ cloned %s → %s (%s)\n  %s\n", p.Title(), c.Title(), id, a.Target(c, false, false).ShellCommand())
	return nil
}

func runMerge(cfg app.Config, args []string) error {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	name := fs.String("name", "", "title for the new pup")
	cwd := fs.String("cwd", "", "folder to start in (default: the first session's)")
	kind := fs.String("opener", "", "herdr | ghostty | inline | print")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("usage: kennel merge <ref> <ref>... | kennel distill <ref>")
	}
	if *kind != "" {
		cfg.Opener = opener.Kind(*kind)
	}
	a, err := openApp(cfg, true)
	if err != nil {
		return err
	}
	defer a.Close()
	var ids []string
	for _, ref := range fs.Args() {
		p := a.Find(ref)
		if p == nil {
			return fmt.Errorf("no session matches %q", ref)
		}
		ids = append(ids, p.ID)
	}
	t, err := a.MergeTarget(ids, app.ExpandHome(*cwd), *name)
	if err != nil {
		return err
	}
	if opener.Resolve(cfg.Opener) == opener.Inline {
		c := opener.InlineCmd(t)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	}
	msg, err := a.Opener().Open(t)
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}
