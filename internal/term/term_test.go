package term

import (
	"strings"
	"testing"
	"time"
)

func TestTermRoundTrip(t *testing.T) {
	m := NewManager()
	tm, err := m.Start("s1", t.TempDir(), "/bin/sh", []string{"-c", "echo ready; exec cat"}, nil, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := m.Start("s1", "/", "/bin/false", nil, nil, 80, 24); again != tm {
		t.Fatal("a running session keeps its terminal")
	}
	read := func(want string) {
		deadline := time.After(3 * time.Second)
		for {
			back, _, done := tm.Attach()
			done()
			if strings.Contains(string(back), want) {
				return
			}
			select {
			case <-deadline:
				t.Fatalf("never saw %q in %q", want, back)
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	read("ready")
	_, live, detach := tm.Attach()
	if err := tm.Write([]byte("hello pty\n")); err != nil {
		t.Fatal(err)
	}
	got := ""
	for !strings.Contains(got, "hello pty") {
		select {
		case b := <-live:
			got += string(b)
		case <-time.After(3 * time.Second):
			t.Fatalf("no echo, got %q", got)
		}
	}
	detach()
	if err := tm.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	tm.Stop()
	deadline := time.After(3 * time.Second)
	for m.Get("s1") != nil {
		select {
		case <-deadline:
			t.Fatal("terminal should be gone after stop")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
