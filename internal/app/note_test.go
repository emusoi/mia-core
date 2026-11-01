package app

import "testing"

func TestTheNoteNamesEveryCheckoutAndEachRepositoryHasItsOwn(t *testing.T) {
	got := checkouts([]Listing{
		{Name: "learnharness", Main: true, Adopted: true, Branch: "feature/wazo-agent"},
		{Name: "oloirien", Adopted: true},
		{Name: "app.wk3"}, // made by hand, never adopted: not mia's to offer
	})
	if len(got) != 2 || got[0].Name != "learnharness" {
		t.Errorf("the main checkout, where the work is, is in the note: %+v", got)
	}
	if NotePath("src/emusoi/learnharness") == NotePath("src/emusoi/mia-core") {
		t.Error("two repositories, two notes")
	}
}
