package transport

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Baseline pins published field numbers/names/types/cardinality and RPC input /
// output identities. Additions are allowed; incompatible edits fail explicitly.
func contractShape() map[string]string {
	out := map[string]string{}
	f := pb.File_dream_v1_service_proto
	for i := 0; i < f.Messages().Len(); i++ {
		m := f.Messages().Get(i)
		for j := 0; j < m.Fields().Len(); j++ {
			field := m.Fields().Get(j)
			name := string(field.FullName())
			value := field.Kind().String() + "/" + field.Cardinality().String() + "/" + field.JSONName()
			if field.Kind() == protoreflect.MessageKind {
				value += "/" + string(field.Message().FullName())
			}
			out[name] = value + "/" + strconv.Itoa(int(field.Number()))
		}
	}
	for i := 0; i < f.Services().Len(); i++ {
		s := f.Services().Get(i)
		for j := 0; j < s.Methods().Len(); j++ {
			m := s.Methods().Get(j)
			out[string(m.FullName())] = string(m.Input().FullName()) + "->" + string(m.Output().FullName())
		}
	}
	return out
}
func TestWireContractCompatibility(t *testing.T) {
	current := contractShape()
	raw, err := os.ReadFile("testdata/contract-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var baseline map[string]string
	if err = json.Unmarshal(raw, &baseline); err != nil {
		t.Fatal(err)
	}
	for key, value := range baseline {
		if current[key] != value {
			t.Errorf("breaking wire change: %s: was %s, now %s", key, value, current[key])
		}
	}
}
