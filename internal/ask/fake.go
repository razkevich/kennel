package ask

import (
	"context"
	"strings"
)

// Fake answers without calling Claude; used by demos and end-to-end tests (KENNEL_FAKE_LLM=1).
type Fake struct{}

func (Fake) Complete(_ context.Context, _ string, prompt string, onText func(string)) (string, error) {
	if strings.Contains(prompt, "Propose 8-14 short themes") {
		return "Payments, Infra, Mobile, Search, Tooling, Incidents", nil
	}
	if strings.Contains(prompt, "into 2-5 floors of a building") {
		var work, rest []string
		for _, line := range strings.Split(prompt, "\n") {
			name, _, ok := strings.Cut(strings.TrimPrefix(line, "- "), " (e.g.")
			if !ok || !strings.HasPrefix(line, "- ") {
				continue
			}
			if strings.Contains(strings.ToLower(line), "payment") || strings.Contains(strings.ToLower(line), "infra") {
				work = append(work, name)
			} else {
				rest = append(rest, name)
			}
		}
		return "Work: " + strings.Join(work, ", ") + "\nEverything else: " + strings.Join(rest, ", "), nil
	}
	if strings.Contains(prompt, "Assign each coding-assistant session") {
		var out []string
		for _, line := range strings.Split(prompt, "\n") {
			id, rest, ok := strings.Cut(line, " | ")
			if !ok || len(id) != 8 {
				continue
			}
			l := strings.ToLower(rest)
			theme := "Tooling"
			switch {
			case strings.Contains(l, "checkout") || strings.Contains(l, "payment") || strings.Contains(l, "invoice") || strings.Contains(l, "rate"):
				theme = "Payments"
			case strings.Contains(l, "incident") || strings.Contains(l, "crash") || strings.Contains(l, "flaky"):
				theme = "Incidents"
			case strings.Contains(l, "postgres") || strings.Contains(l, "terraform") || strings.Contains(l, "cost") || strings.Contains(l, "infra"):
				theme = "Infra"
			case strings.Contains(l, "ios") || strings.Contains(l, "mobile") || strings.Contains(l, "dark mode") || strings.Contains(l, "widget"):
				theme = "Mobile"
			case strings.Contains(l, "search"):
				theme = "Search"
			}
			out = append(out, id+": "+theme)
		}
		return strings.Join(out, "\n"), nil
	}
	if strings.Contains(prompt, "names workstreams") {
		return "STRIPE-FLICKER", nil
	}
	if strings.Contains(prompt, "search keywords") {
		return "stripe, PaymentElement, flicker, postgres, upgrade, replication", nil
	}
	answer := "From your sessions: the checkout flicker came from remounting the payment element, fixed by memoizing its options [1]. The Postgres 16 plan uses logical replication with a short write freeze [2]."
	if !strings.Contains(prompt, "[2]") {
		answer = "Closest match: the checkout session fixed a flicker by memoizing the payment element options [1]."
	}
	for _, w := range strings.SplitAfter(answer, " ") {
		if onText != nil {
			onText(w)
		}
	}
	return answer, nil
}
