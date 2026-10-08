package app_test

import (
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

func TestHostRecordsThePlacement(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	record, err := a.New("hosted")
	if err != nil {
		t.Fatal(err)
	}

	hosted, err := a.Host(record, runtime.Machine{Name: runtime.Local})
	if err != nil {
		t.Fatal(err)
	}
	if hosted.Env == nil || hosted.Env.Placement != model.Local() {
		t.Errorf("placement %+v, want local", hosted.Env)
	}

	records, _ := a.Store.Load()
	if len(records) != 1 || records[0].Env == nil || records[0].Env.Placement != model.Local() {
		t.Errorf("the record on disk did not follow: %+v", records)
	}
}
