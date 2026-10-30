package cli

import (
	"sync"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/panel"
)

func TestAReloadNeverSeesTheListingForgottenUnderIt(t *testing.T) {
	var c listingCache
	list := func() ([]app.Listing, error) {
		time.Sleep(time.Millisecond)
		return []app.Listing{{Name: "main", Main: true}}, nil
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				c.forget()
			}
		}
	}()
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				got, err := c.get(list)
				if err != nil || len(got) != 1 {
					t.Errorf("a reload saw %d worktrees, %v", len(got), err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
}

func TestNewSaysWhereANewBranchStarts(t *testing.T) {
	p := panel.Panel{Actions: map[string]panel.Action{"new": {Key: "n", Label: "new"}, "branch": {Key: "+", Label: "new worktree"}}}
	namesBase(p, "", []app.Listing{{Name: "lido-app", Branch: "staging", Main: true}, {Name: "temeke", Branch: "auth"}})
	if got := p.Actions["new"].Label; got != "new worktree from staging" {
		t.Errorf("the main checkout's branch, as mia new starts from it: %q", got)
	}
	namesBase(p, "production", nil)
	if got := p.Actions["branch"].Label; got != "new worktree from production, and go there" {
		t.Errorf("a configured base wins: %q", got)
	}
}
