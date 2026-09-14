package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tushardhara/dream/simulator/experiment"
)

func TestRelationshipWorkerRejectsEvaluatorInputs(t *testing.T) {
	r := experiment.RelationshipProbeRequest{Version: experiment.RelationshipExperimentVersion, FixtureHash: experiment.RelationshipFixtureHash(), Seeds: []uint64{11, 23}}
	raw, _ := json.Marshal(r)
	var out bytes.Buffer
	if e := runRelationshipProbe(bytes.NewReader(raw), &out); e != nil {
		t.Fatal(e)
	}
	var result experiment.RelationshipProbe
	if e := json.Unmarshal(out.Bytes(), &result); e != nil || result.Validate(r) != nil {
		t.Fatal("invalid probe output", e)
	}
	for _, key := range []string{"labels", "private_state", "dataset", "environment", "source_path", "hypothesis", "decision_rule"} {
		bad := strings.TrimSuffix(string(raw), "}") + `,"` + key + `":"EVALUATOR_ONLY_CANARY"}`
		out.Reset()
		if e := runRelationshipProbe(strings.NewReader(bad), &out); e == nil || out.Len() != 0 {
			t.Fatal("evaluator/private input reached generation", key)
		}
	}
	for _, bad := range []string{string(raw) + " {}", `{"version":"relationship-reference-experiment.v1","seeds":[11,11]}`, `[]`} {
		if e := runRelationshipProbe(strings.NewReader(bad), &out); e == nil {
			t.Fatal("invalid probe envelope accepted")
		}
	}
}
