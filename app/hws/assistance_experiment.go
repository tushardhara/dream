package hws

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
)

const AssistanceExperimentVersion = "helper-experiment.v1"

type AssistanceManifest struct {
	Version, WorldVersion, HelperVersion, FixtureHash string
	Arm                                               assistance.Arm
	HumanSeed, ExogenousSeed, HelperSeed              uint64
}
type HumanFrame struct {
	Actor     core.ID
	At        core.LogicalTime
	Situation behavior.ActionSituation
}

// ExogenousResource is a bounded external availability shock, fixed before any
// human/helper decision. It is simulator-only and never passed to the helper.
type ExogenousResource struct {
	Frame              int
	Resource           core.ID
	MinUnits, MaxUnits int64
}
type RealizedResource struct {
	Frame    int
	Resource core.ID
	Units    int64
	Draw     uint64
}
type AssistanceWorld struct {
	Exogenous []ExogenousResource
	Actors    []behavior.ActionActor
	Frames    []HumanFrame
}
type AssistanceRun struct {
	Exogenous []RealizedResource
	Manifest  AssistanceManifest
	Humans    []behavior.ActionDecision
	Helper    []assistance.Interaction
	Final     []behavior.ActionActor
	// Index of the first human frame offered an actual delivered intervention.
	// -1 means no delivered coordination proposal; divergence is not presumed.
	FirstIntervention int
}

// AssistanceStep is composed by a trusted host. It receives only a clock/ordinal,
// not private human state, decisions, future frames or evaluator labels.
type AssistanceStep interface {
	Step(context.Context, int, core.LogicalTime, uint64, *assistance.Interaction) (assistance.Interaction, error)
}

func streamDraw(domain string, seed uint64, ordinal int) uint64 {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%d", domain, seed, ordinal)))
	return binary.BigEndian.Uint64(h[:8])
}
func NewAssistanceManifest(world AssistanceWorld, arm assistance.Arm, seed uint64) AssistanceManifest {
	return AssistanceManifest{Version: AssistanceExperimentVersion, WorldVersion: behavior.ActionPolicy, HelperVersion: assistance.Version, FixtureHash: assistance.Digest(world), Arm: arm, HumanSeed: streamDraw("human", seed, 0), ExogenousSeed: streamDraw("exogenous", seed, 0), HelperSeed: streamDraw("helper", seed, 0)}
}

// RunAssistance keeps the same v2 human choice engine in every arm, including
// no-assistant. A coordination proposal adds an affordance to its recipient's next
// frame; it neither selects their action nor supplies an observed positive outcome.
func RunAssistance(ctx context.Context, world AssistanceWorld, m AssistanceManifest, helper AssistanceStep, recorded *AssistanceRun) (AssistanceRun, error) {
	if m.Version != AssistanceExperimentVersion || m.WorldVersion != behavior.ActionPolicy || m.HelperVersion != assistance.Version || !m.Arm.Valid() || m.FixtureHash != assistance.Digest(world) || len(world.Actors) < 2 || len(world.Actors) > 8 || len(world.Frames) < 1 || len(world.Frames) > 16 || len(world.Exogenous) > 16 || helper == nil {
		return AssistanceRun{}, assistance.ErrInvalid
	}
	if recorded != nil && (assistance.Digest(recorded.Manifest) != assistance.Digest(m) || len(recorded.Helper) != len(world.Frames)) {
		return AssistanceRun{}, assistance.ErrInvalid
	}
	raw, _ := json.Marshal(world)
	var owned AssistanceWorld
	_ = json.Unmarshal(raw, &owned)
	actors := map[core.ID]int{}
	for i, a := range owned.Actors {
		if a.Validate() != nil {
			return AssistanceRun{}, assistance.ErrInvalid
		}
		if _, ok := actors[a.Drives.Actor]; ok {
			return AssistanceRun{}, assistance.ErrInvalid
		}
		actors[a.Drives.Actor] = i
	}
	out := AssistanceRun{Manifest: m, FirstIntervention: -1}
	seenExogenous := map[struct {
		Frame    int
		Resource core.ID
	}]bool{}
	for i, event := range owned.Exogenous {
		key := struct {
			Frame    int
			Resource core.ID
		}{event.Frame, event.Resource}
		if event.Frame < 0 || event.Frame >= len(owned.Frames) || event.Resource.Validate() != nil || event.MinUnits < 0 || event.MaxUnits < event.MinUnits || event.MaxUnits > 1000000 || seenExogenous[key] {
			return AssistanceRun{}, assistance.ErrInvalid
		}
		seenExogenous[key] = true
		draw := streamDraw("exogenous-resource.v1", m.ExogenousSeed, i)
		units := event.MinUnits + int64(draw%uint64(event.MaxUnits-event.MinUnits+1))
		if owned.Frames[event.Frame].Situation.Resources == nil {
			owned.Frames[event.Frame].Situation.Resources = map[core.ID]int64{}
		}
		owned.Frames[event.Frame].Situation.Resources[event.Resource] = units
		out.Exogenous = append(out.Exogenous, RealizedResource{Frame: event.Frame, Resource: event.Resource, Units: units, Draw: draw})
	}
	pending := map[core.ID]bool{}
	var previous core.LogicalTime
	for i, f := range owned.Frames {
		if ctx.Err() != nil {
			return AssistanceRun{}, ctx.Err()
		}
		index, ok := actors[f.Actor]
		if !ok || f.At < previous {
			return AssistanceRun{}, assistance.ErrInvalid
		}
		previous = f.At
		s := f.Situation
		if pending[f.Actor] {
			// The fixed helper template is an invitation to consider coordination, not
			// disclosure of private source material. The human chooses using own evidence.
			recipient := core.ID("")
			for _, id := range s.Present {
				if id != f.Actor {
					recipient = id
					break
				}
			}
			if recipient != "" {
				s.Offers = append(s.Offers, behavior.ActionOffer{Kind: behavior.Coordinate, Recipient: recipient, Evidence: []core.ID{s.Observation.Event.Event}, Duration: 1})
				if out.FirstIntervention == -1 {
					out.FirstIntervention = i
				}
			}
			delete(pending, f.Actor)
		}
		next, decision, e := behavior.ChooseAction(owned.Actors[index], s, f.At, streamDraw("human-choice.v1", m.HumanSeed, i))
		if e != nil {
			return AssistanceRun{}, e
		}
		owned.Actors[index] = next
		out.Humans = append(out.Humans, decision)
		var record *assistance.Interaction
		if recorded != nil {
			record = &recorded.Helper[i]
		}
		interaction, e := helper.Step(ctx, i, f.At, streamDraw("helper-choice.v1", m.HelperSeed, i), record)
		if e != nil {
			return AssistanceRun{}, e
		}
		if interaction.Arm != m.Arm || interaction.Seed != streamDraw("helper-choice.v1", m.HelperSeed, i) || interaction.Version != assistance.Version || interaction.At != f.At || len(interaction.Result.Candidates) == 0 || interaction.Result.Selected < 0 || interaction.Result.Selected >= len(interaction.Result.Candidates) {
			return AssistanceRun{}, assistance.ErrInvalid
		}
		if _, ok := actors[interaction.Helper]; ok {
			return AssistanceRun{}, assistance.ErrInvalid
		}
		if _, ok := actors[interaction.User]; !ok {
			return AssistanceRun{}, assistance.ErrInvalid
		}
		selected := interaction.Result.Candidates[interaction.Result.Selected]
		if m.Arm == assistance.None && (interaction.Delivered || selected.Action != assistance.Wait) {
			return AssistanceRun{}, assistance.ErrInvalid
		}
		if interaction.Delivered && selected.Action == assistance.Propose {
			if _, ok := actors[selected.Recipient]; !ok {
				return AssistanceRun{}, assistance.ErrInvalid
			}
			pending[selected.Recipient] = true
		}
		out.Helper = append(out.Helper, interaction)
	}
	out.Final = owned.Actors
	if recorded != nil && assistance.Digest(out) != assistance.Digest(*recorded) {
		return AssistanceRun{}, assistance.ErrInvalid
	}
	return out, nil
}
