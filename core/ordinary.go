package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const OrdinaryVersion = "ordinary-life.v1"

type OrdinaryKind string

const (
	OrdinaryQuiet        OrdinaryKind = "quiet"
	OrdinaryAppreciation OrdinaryKind = "appreciation"
	OrdinaryMemory       OrdinaryKind = "shared_memory"
	OrdinaryActivity     OrdinaryKind = "simple_activity"
	OrdinaryCoordination OrdinaryKind = "requested_coordination"
)

func (k OrdinaryKind) Valid() bool {
	return k == OrdinaryQuiet || k == OrdinaryAppreciation || k == OrdinaryMemory || k == OrdinaryActivity || k == OrdinaryCoordination
}

// ExpectedBenefit is a participant's reported expectation, not an observed future
// outcome. Unknown capacity/benefit never becomes zero or permission to interrupt.
type OrdinaryPreference struct {
	Version                    string
	Source, Person, Activity   ID
	Kind                       OrdinaryKind
	Window                     Interval
	Choice, Availability       string
	ExpectedBenefit, MaxEffort GroupQuantity
	Corrects                   ID `json:",omitempty"`
}

func (p OrdinaryPreference) Validate() error {
	if p.Version != OrdinaryVersion || ids(p.Source, p.Person, p.Activity) != nil || !p.Kind.Valid() || p.Window.Validate() != nil || p.Window.End == nil {
		return fmt.Errorf("invalid ordinary preference identity/window")
	}
	if p.Choice != "wanted" && p.Choice != "declined" && p.Choice != "unknown" {
		return fmt.Errorf("invalid declared ordinary preference")
	}
	if p.Availability != "available" && p.Availability != "busy" && p.Availability != "unknown" {
		return fmt.Errorf("invalid reported availability")
	}
	if p.ExpectedBenefit.Validate(-1, 1) != nil || p.MaxEffort.Validate(0, 100) != nil {
		return fmt.Errorf("invalid ordinary expected benefit/effort")
	}
	if p.Corrects != "" && (p.Corrects.Validate() != nil || p.Corrects == p.Source) {
		return fmt.Errorf("invalid ordinary preference correction")
	}
	return nil
}

// Words are submitted by the authenticated author. A host cannot establish real
// authenticity from a flag: it must bind Author to submission authority. AI text
// has no accepted origin and cannot be attributed to a participant by this type.
type OrdinaryStory struct {
	Version                  string
	Source, Author, Activity ID
	Kind                     OrdinaryKind
	Origin, Words            string
	Corrects                 ID `json:",omitempty"`
}

func (s OrdinaryStory) Validate() error {
	if s.Version != OrdinaryVersion || ids(s.Source, s.Author, s.Activity) != nil || s.Kind != OrdinaryAppreciation && s.Kind != OrdinaryMemory {
		return fmt.Errorf("invalid ordinary story identity/kind")
	}
	if s.Origin != "participant_authored" {
		return fmt.Errorf("ordinary words require participant authorship")
	}
	if !utf8.ValidString(s.Words) || strings.TrimSpace(s.Words) == "" || len(s.Words) > 512 {
		return fmt.Errorf("invalid authored ordinary words")
	}
	if s.Corrects != "" && (s.Corrects.Validate() != nil || s.Corrects == s.Source) {
		return fmt.Errorf("invalid ordinary story correction")
	}
	return nil
}
func EncodeOrdinaryPreference(p OrdinaryPreference) ([]byte, error) {
	if e := p.Validate(); e != nil {
		return nil, e
	}
	return json.Marshal(p)
}
func EncodeOrdinaryStory(s OrdinaryStory) ([]byte, error) {
	if e := s.Validate(); e != nil {
		return nil, e
	}
	return json.Marshal(s)
}
func decodeOrdinary[T any](raw []byte, out *T) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 2048 || d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid ordinary encoding")
	}
	return nil
}
func DecodeOrdinaryPreference(raw []byte) (OrdinaryPreference, error) {
	var p OrdinaryPreference
	if e := decodeOrdinary(raw, &p); e != nil {
		return p, e
	}
	return p, p.Validate()
}
func DecodeOrdinaryStory(raw []byte) (OrdinaryStory, error) {
	var s OrdinaryStory
	if e := decodeOrdinary(raw, &s); e != nil {
		return s, e
	}
	return s, s.Validate()
}

// This is a separate later self-report. Neither an invitation nor a reply is
// an observation of welcome or reduced burden. The host authenticates Participant
// and binds Opportunity to a previously recorded interaction, including WAIT.
type OrdinaryExperience struct {
	Version                                        string
	ID, Opportunity, Participant, Observer, Source ID
	OccurredAt, LearnedAt                          LogicalTime
	Participation                                  string
	Benefit, BurdenReduction                       GroupQuantity
}

func (e OrdinaryExperience) Validate() error {
	if e.Version != OrdinaryVersion || ids(e.ID, e.Opportunity, e.Participant, e.Observer, e.Source) != nil || e.Participant != e.Observer || e.Source != e.Participant || e.OccurredAt < 0 || e.LearnedAt < e.OccurredAt {
		return fmt.Errorf("invalid ordinary experience attribution/time")
	}
	if e.Participation != "unknown" && e.Participation != "welcomed" && e.Participation != "unwelcome" && e.Participation != "declined" {
		return fmt.Errorf("invalid observed ordinary participation")
	}
	if e.Benefit.Validate(-1, 1) != nil || e.BurdenReduction.Validate(-100, 100) != nil {
		return fmt.Errorf("invalid ordinary experience quantity")
	}
	return nil
}
