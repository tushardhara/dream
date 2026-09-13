package evaluation

import (
	"context"
	"os"
	"testing"

	"github.com/tushardhara/dream/simulator/experiment"
)

func TestContainerPermissionProbe(t *testing.T) {
	image := os.Getenv("DREAM_EVALUATOR_PROBE_IMAGE")
	if image == "" {
		t.Skip("requires disposable isolation probe image")
	}
	// The probe exits nonzero if it can read evaluator files/environment, contact
	// the host listener, gain root, or write its root filesystem. Only [] is success.
	result, e := (Container{Image: image}).Generate(context.Background(), []experiment.Request{{}})
	if e != nil || len(result) != 0 {
		t.Fatal("generator permissions isolation failed", e)
	}
}
