package app

import (
	"sync"
	"testing"
)

// UI goroutines read pups while the live loop and metadata edits run.
func TestConcurrentReadsAndRefresh(t *testing.T) {
	a := demoApp(t)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, p := range a.Pups() {
				_ = p.Status.String() + p.Pane + p.Title()
				_ = p.Meta.Starred
			}
		}
	}()
	for i := 0; i < 50; i++ {
		a.RefreshLive(false)
		if err := a.ToggleStar(a.Pups()[i%5].ID); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}
