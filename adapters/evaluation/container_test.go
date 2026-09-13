package evaluation

import (
	"context"
	"os"
	"testing"

	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/simulator/experiment"
)

func TestContainerRejectsTagsAndOutputBudget(t *testing.T) {
	for _, image := range []string{"latest", "repo:tag", "--privileged", "sha256:bad"} {
		if _, e := (Container{Image: image}).Generate(context.Background(), []experiment.Request{{}}); e == nil {
			t.Fatal("unpinned image accepted")
		}
	}
	var output limitedOutput
	for range 32 {
		if _, e := output.Write(make([]byte, 1<<20)); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := output.Write([]byte("x")); e == nil || !output.overflow {
		t.Fatal("unbounded child output")
	}
}
func TestIsolatedReferenceMatchesPureGeneration(t *testing.T) {
	image := os.Getenv("DREAM_EVALUATOR_TEST_IMAGE")
	if image == "" {
		t.Skip("requires scripts/evaluation-check.py disposable image")
	}
	d, _, e := evals.SyntheticFixture()
	if e != nil {
		t.Fatal(e)
	}
	requests := []experiment.Request{}
	for _, v := range experiment.Variants() {
		requests = append(requests, experiment.Request{Input: d.Cases[0].Input, Variant: v, Seed: 11})
	}
	want, e := (experiment.Generator{}).Generate(context.Background(), requests)
	if e != nil {
		t.Fatal(e)
	}
	got, e := (Container{Image: image}).Generate(context.Background(), requests)
	if e != nil {
		t.Fatal(e)
	}
	w, _ := evals.Digest(want)
	g, _ := evals.Digest(got)
	if w != g {
		t.Fatal("isolated predictions differ")
	}
}
