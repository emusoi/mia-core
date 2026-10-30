package cli

import (
	"sync"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
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
