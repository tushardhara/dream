package graph

import (
	"math"
	"testing"
	"time"

	"github.com/tushardhara/dream/core"
)

func TestDigestCanonicalSetsAndZero(t *testing.T) {
	c := AppendCommand{Actor: "a", Namespace: "n", Operation: "append", Key: "k", Class: PrivatePayload, Payload: Payload{Version: 1, Text: "synthetic"}, Event: core.Event{Version: 1, Stream: "s", Type: "observation", Subject: core.Subject{Principal: "a"}, Meta: core.Metadata{ID: "e", Observer: "a", Source: "a", Sensitivity: core.Restricted, Parents: []core.ID{"z", "b"}, Confidence: core.Confidence(math.Copysign(0, -1)), RecordedAt: time.Unix(1, 0), Rights: core.Rights{Resource: "e"}}}}
	first, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	c.Event.Meta.Parents = []core.ID{"b", "z"}
	c.Event.Meta.Confidence = 0
	c.Event.Meta.RecordedAt = time.Unix(2, 0)
	second, err := c.Digest()
	if err != nil || first != second {
		t.Fatal("logical command digest depends on order/zero/wall time", err)
	}
}
