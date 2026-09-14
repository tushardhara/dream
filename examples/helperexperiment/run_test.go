package helperexperiment

import (
	"context"
	"testing"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/examples/assistanceclient"
	"github.com/tushardhara/dream/simulator/behavior"
)

func TestMatchedArmsReplayAndHumanActivity(t *testing.T) {
	seen := map[behavior.Kind]bool{}
	for seed := uint64(1); seed <= 12; seed++ {
		var baseline hws.AssistanceRun
		for _, arm := range []assistance.Arm{assistance.None, assistance.Simple, assistance.Single, assistance.Multi} {
			out, e := Run(context.Background(), arm, assistance.Coordinate, seed, nil)
			if e != nil {
				t.Fatal(arm, e)
			}
			replay, e := Run(context.Background(), arm, assistance.Coordinate, seed, &out)
			if e != nil || assistance.Digest(out) != assistance.Digest(replay) {
				t.Fatal("recorded replay", arm, e)
			}
			if len(out.Humans) != 8 || len(out.Helper) != 8 {
				t.Fatal("human engine suppressed")
			}
			if arm == assistance.None {
				baseline = out
				for _, d := range out.Humans {
					seen[d.Candidates[d.Selected].Offer.Kind] = true
				}
				for _, h := range out.Helper {
					if h.Delivered {
						t.Fatal("disabled helper delivered")
					}
				}
				if out.FirstIntervention != -1 {
					t.Fatal("disabled arm intervened")
				}
			} else {
				if out.Manifest.HumanSeed != baseline.Manifest.HumanSeed || out.Manifest.ExogenousSeed != baseline.Manifest.ExogenousSeed || out.Manifest.FixtureHash != baseline.Manifest.FixtureHash {
					t.Fatal("unmatched worlds")
				}
				if out.FirstIntervention != 2 {
					t.Fatal("intervention not applied to next user turn", out.FirstIntervention)
				}
				for i := 0; i < out.FirstIntervention; i++ {
					if assistance.Digest(out.Humans[i]) != assistance.Digest(baseline.Humans[i]) {
						t.Fatal("human diverged before actual intervention")
					}
				}
			}
		}
	}
	for _, kind := range []behavior.Kind{behavior.Say, behavior.Argue, behavior.Help, behavior.Wait} {
		if !seen[kind] {
			t.Fatal("no-assistant removed human behavior", kind)
		}
	}
}
func TestHelperWaitDoesNotChangeWorldAndReplayRejectsDrift(t *testing.T) {
	baseline, e := Run(context.Background(), assistance.None, assistance.Pause, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, arm := range []assistance.Arm{assistance.Simple, assistance.Single, assistance.Multi} {
		out, e := Run(context.Background(), arm, assistance.Pause, 11, nil)
		if e != nil {
			t.Fatal(e)
		}
		if assistance.Digest(out.Humans) != assistance.Digest(baseline.Humans) || out.FirstIntervention != -1 {
			t.Fatal("helper draw/order changed human world while WAIT")
		}
		bad := out
		bad.Manifest.HelperVersion = "future"
		if _, e := Run(context.Background(), arm, assistance.Pause, 11, &bad); e == nil {
			t.Fatal("version drift replayed")
		}
		if _, e := Run(context.Background(), arm, assistance.Pause, 12, &out); e == nil {
			t.Fatal("seed drift replayed")
		}
	}
}

func TestChangingOnlyHelperSeedLeavesWaitingHumansUnchanged(t *testing.T) {
	world := World()
	var first hws.AssistanceRun
	for i := 0; i < 2; i++ {
		local, r := assistanceclient.Fixture(assistance.Multi, assistance.Pause)
		s := &steps{local: local, request: r, host: local.Host(assistance.FakePlanner{})}
		m := hws.NewAssistanceManifest(world, assistance.Multi, 11)
		m.HelperSeed += uint64(i)
		out, e := hws.RunAssistance(context.Background(), world, m, s, nil)
		if e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			first = out
		} else if assistance.Digest(out.Humans) != assistance.Digest(first.Humans) || out.Helper[0].Seed == first.Helper[0].Seed {
			t.Fatal("helper stream coupled to human stream")
		}
	}
}
