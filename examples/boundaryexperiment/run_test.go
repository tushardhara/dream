package boundaryexperiment

import (
	"context"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/examples/assistanceclient"
	"github.com/tushardhara/dream/examples/helperexperiment"
	"github.com/tushardhara/dream/simulator/behavior"
	"testing"
)

func TestRealScopedHelperAndHumanConsumerReplay(t *testing.T) {
	for _, signal := range []string{"disagreement", "credible_pressure"} {
		out, e := Run(context.Background(), signal, assistance.Single, nil)
		if e != nil {
			t.Fatal(e)
		}
		if out.Helpers[0].Delivered != (signal == "disagreement") || !out.Helpers[1].Delivered {
			t.Fatal("pressure or unrelated helper control", out.Helpers)
		}
		for i, want := range []behavior.Kind{behavior.Leave, behavior.Say} {
			d := out.Human[i].Human
			if d.Candidates[d.Selected].Offer.Kind != want {
				t.Fatal("scoped human consumer", i, d)
			}
		}
		replay, e := Run(context.Background(), signal, assistance.Single, &out)
		if e != nil || assistance.Digest(out) != assistance.Digest(replay) {
			t.Fatal("mechanical replay", e)
		}
		out.Human[1].BoundaryHash = "tampered"
		if _, e := Run(context.Background(), signal, assistance.Single, &out); e == nil {
			t.Fatal("tampered scoped replay")
		}
	}
}
func TestFrozen50CodecsAndExperiment(t *testing.T) {
	l, r := assistanceclient.Fixture(assistance.Single, assistance.Coordinate)
	interaction, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
	if e != nil {
		t.Fatal(e)
	}
	run, e := helperexperiment.Run(context.Background(), assistance.Multi, assistance.Coordinate, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	actor, e := behavior.NewActionActor("alice", 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct {
		value any
		hash  string
	}{
		{actor, "84e2a18487197aaf36442e65386d992c1ca9964c1846f83105e9cd2022fa601c"},
		{run, "c03e4308dadff8c2982b74f0baa1001da93b324bfd2cca8bd9f3af282295bf40"},
		{interaction, "1993b78df85f4bcdd00278492b7bb0b56aa055a102824037c7b1cc576d5f5749"},
		{r, "64b62068b61ec8380452859c9dc186c5fb3864d7f118f0cb7d89f9db951a1fc4"},
	} {
		if assistance.Digest(v.value) != v.hash {
			t.Fatal("frozen50 wire/hash changed")
		}
	}
}
