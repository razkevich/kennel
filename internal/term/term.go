// Package term runs agent sessions in pseudo-terminals the web page can attach to:
// the "terminal" under a selected pup. Output is kept in a short scrollback so a
// page that attaches later (or reloads) sees what happened.
package term

import (
	"errors"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

const keep = 256 << 10

// Term is one running program in a pty.
type Term struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	pty    *os.File
	buf    []byte
	subs   map[chan []byte]bool
	exited bool
}

// Manager holds the terminals, one per session id.
type Manager struct {
	mu    sync.Mutex
	terms map[string]*Term
}

func NewManager() *Manager { return &Manager{terms: map[string]*Term{}} }

// Get returns the session's terminal while its program runs.
func (m *Manager) Get(id string) *Term {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.terms[id]; t != nil && !t.Exited() {
		return t
	}
	return nil
}

// Start runs name+args in dir under a new pty, unless the session already has one.
func (m *Manager) Start(id, dir, name string, args []string, env []string, cols, rows int) (*Term, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.terms[id]; t != nil && !t.Exited() {
		return t, nil
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append([]string{"TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=kennel"}, env...)...)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(max(cols, 20)), Rows: uint16(max(rows, 5))})
	if err != nil {
		return nil, err
	}
	t := &Term{cmd: cmd, pty: f, subs: map[chan []byte]bool{}}
	m.terms[id] = t
	go t.pump()
	return t, nil
}

func (t *Term) pump() {
	b := make([]byte, 32<<10)
	for {
		n, err := t.pty.Read(b)
		if n > 0 {
			chunk := append([]byte(nil), b[:n]...)
			t.mu.Lock()
			t.buf = append(t.buf, chunk...)
			if len(t.buf) > keep {
				t.buf = t.buf[len(t.buf)-keep:]
			}
			for c := range t.subs {
				select {
				case c <- chunk:
				default: // a slow page loses output rather than stalling the program
				}
			}
			t.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	_ = t.cmd.Wait()
	t.mu.Lock()
	t.exited = true
	for c := range t.subs {
		close(c)
	}
	t.subs = map[chan []byte]bool{}
	t.mu.Unlock()
	t.pty.Close()
}

// Attach returns the scrollback so far and a channel of what comes next; the
// channel closes when the program exits. Call the returned func to detach.
func (t *Term) Attach() ([]byte, <-chan []byte, func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	c := make(chan []byte, 256)
	back := append([]byte(nil), t.buf...)
	if t.exited {
		close(c)
		return back, c, func() {}
	}
	t.subs[c] = true
	return back, c, func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.subs[c] {
			delete(t.subs, c)
			close(c)
		}
	}
}

func (t *Term) Write(p []byte) error {
	if t.Exited() {
		return errors.New("the program has exited")
	}
	_, err := t.pty.Write(p)
	return err
}

func (t *Term) Resize(cols, rows int) error {
	if t.Exited() {
		return nil
	}
	return pty.Setsize(t.pty, &pty.Winsize{Cols: uint16(max(cols, 20)), Rows: uint16(max(rows, 5))})
}

func (t *Term) Exited() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exited
}

// Stop ends the program: an interrupt, then closing the pty hangs it up like
// closing a terminal window.
func (t *Term) Stop() {
	if !t.Exited() && t.cmd.Process != nil {
		_ = t.cmd.Process.Signal(os.Interrupt)
		t.pty.Close()
	}
}

// StopAll ends every running terminal, when kennel shuts down.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.terms {
		t.Stop()
	}
}
