package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
)

// Tell hands a pup one message without opening it: typed into its herdr pane when
// it's running there, or as a headless resumed turn when it's an asleep Claude
// session. The headless turn's output goes to ~/.kennel/tell/<id>.log.
func (a *App) Tell(p *pack.Pup, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("say something first")
	}
	if p.Status.Live() {
		if p.Pane == "" {
			return "", fmt.Errorf("%s is running in another terminal, so kennel can't type into it", p.Title())
		}
		if err := opener.Prompt(a.Cfg.Runner, p.Pane, text); err != nil {
			return "", err
		}
		return "💬 told " + p.Title(), nil
	}
	if p.AgentKind() != "claude" {
		return "", fmt.Errorf("%s is a %s session; open it to talk to it", p.Title(), p.AgentKind())
	}
	t := a.Target(p, false, false)
	if err := a.CheckCwd(t.Cwd); err != nil {
		return "", err
	}
	args := []string{"--resume", p.ID, "-p", opener.SafeArg(text)}
	if a.Cfg.Demo {
		return "🐾 would run in the background: claude " + strings.Join(args, " "), nil
	}
	dir := filepath.Join(a.Cfg.Home, "tell")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	log, err := os.OpenFile(filepath.Join(dir, p.ID+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("claude", args...)
	cmd.Dir = t.Cwd
	cmd.Stdout, cmd.Stderr = log, log
	// Its own session, so closing kennel doesn't stop the turn.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		log.Close()
		return "", err
	}
	go func() { _ = cmd.Wait(); log.Close() }()
	return "🐾 " + p.Title() + " is on it in the background", nil
}
