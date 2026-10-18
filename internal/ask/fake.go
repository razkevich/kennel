package ask

import (
	"context"
	"strings"
)

// Fake answers without calling Claude; used by demos and end-to-end tests (KENNEL_FAKE_LLM=1).
type Fake struct{}

func (Fake) Complete(_ context.Context, _ string, prompt string, onText func(string)) (string, error) {
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
