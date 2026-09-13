package graph

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/tushardhara/dream/core"
)

func hashFixture() AppendCommand {
	e := memoryFixture("event", 1).Event
	e.Meta.Parents = []core.ID{"z", "a"}
	e.Meta.Rights.Grants[0], e.Meta.Rights.Grants[2] = e.Meta.Rights.Grants[2], e.Meta.Rights.Grants[0]
	return AppendCommand{Actor: "alice", Namespace: "test", Operation: "put", Key: "event", Event: e, Class: PrivatePayload, Payload: Payload{Version: 1, Text: "synthetic"}}
}
func TestDigestDoesNotMutateInput(t *testing.T) {
	c := hashFixture()
	before, _ := json.Marshal(c)
	first, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(c)
	if string(after) != string(before) {
		t.Fatal("digest mutated caller-owned metadata")
	}
	reordered := hashFixture()
	reordered.Event.Meta.Parents = []core.ID{"a", "z"}
	second, err := reordered.Digest()
	if err != nil || first != second {
		t.Fatal("canonical digest changed", err)
	}
}
func TestLogicalHashDoesNotMutateIndex(t *testing.T) {
	e := memoryFixture("event", 1)
	e.Event.Meta.Rights.Grants[0], e.Event.Meta.Rights.Grants[2] = e.Event.Meta.Rights.Grants[2], e.Event.Meta.Rights.Grants[0]
	idx, err := RebuildMemory(testScope, []MemoryEntry{e})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(idx.entries)
	if _, err = idx.LogicalHash(); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(idx.entries)
	if string(after) != string(before) {
		t.Fatal("logical hash mutated index metadata")
	}
}
func TestConcurrentHashesAndValidation(t *testing.T) {
	c := hashFixture()
	idx, err := RebuildMemory(testScope, []MemoryEntry{memoryFixture("event", 1)})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 50; j++ {
				if _, err := c.Digest(); err != nil {
					errs <- err
					return
				}
				if err := c.Validate(); err != nil {
					errs <- err
					return
				}
				if _, err := idx.LogicalHash(); err != nil {
					errs <- err
					return
				}
			}
			errs <- nil
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
