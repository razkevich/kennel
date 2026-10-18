package pack

import (
	"testing"

	"github.com/razkevich/kennel/internal/claude"
)

func TestRoles(t *testing.T) {
	for want, s := range map[string]claude.Session{
		"firefighter": {FirstPrompt: "p99 alert on checkout, investigate the outage"},
		"reviewer":    {FirstPrompt: "review https://github.com/acme/infra/pull/91"},
		"trader":      {FirstPrompt: "compare kalshi and polymarket odds for the election"},
		"banker":      {FirstPrompt: "why did our GKE billing double last month"},
		"builder":     {FirstPrompt: "write a terraform module for the new bucket"},
		"detective":   {FirstPrompt: "the login e2e test is flaky on CI"},
		"artist":      {FirstPrompt: "make the settings page layout nicer"},
		"mechanic":    {FirstPrompt: "make my zsh startup faster"},
		"":            {FirstPrompt: "hello"},
	} {
		s := s
		p := &Pup{Session: &s}
		if got := p.Role(); got != want {
			t.Errorf("%q: got %q want %q", s.FirstPrompt, got, want)
		}
	}
}
