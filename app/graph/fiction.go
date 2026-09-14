package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/tushardhara/dream/core"
)

// FictionExperience is a bounded synthetic source proposition. Free provider
// prose is neither parsed into an action nor certified as a safe paraphrase.
type FictionExperience struct {
	Version   int     `json:"version"`
	Actor     core.ID `json:"actor"`
	Feeling   string  `json:"feeling"`
	Intensity float64 `json:"intensity"`
}

// FictionDraft selects a deterministic linguistic transform of one already
// approved own-experience proposition. It carries no arbitrary output string.
type FictionDraft struct {
	Version int
	Source  core.ID
	Mode    string
}

func renderFiction(d FictionDraft, safe SafeContext, mode PolicyMode) (ValidatedOutput, PolicyDecision, error) {
	deny := func() (ValidatedOutput, PolicyDecision, error) {
		return ValidatedOutput{}, policyDeny("invalid_fiction_output", -1), nil
	}
	if mode != SyntheticSelfDisclosure || d.Version != 1 || d.Source.Validate() != nil {
		return deny()
	}
	var item SafeContextItem
	found := false
	for _, s := range safe.Items() {
		if s.Source == d.Source {
			item = s
			found = true
		}
	}
	if !found || item.Subject.Principal != item.Observer || item.Subject.Reference != nil {
		return deny()
	}
	var experience FictionExperience
	decoder := json.NewDecoder(bytes.NewBufferString(item.Text))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&experience) != nil || decoder.Decode(new(any)) != io.EOF || experience.Version != 1 || experience.Actor != item.Observer || math.IsNaN(experience.Intensity) || math.IsInf(experience.Intensity, 0) || experience.Intensity < 0 || experience.Intensity > 1 {
		return deny()
	}
	switch experience.Feeling {
	case "lonely", "tired", "worried", "grateful", "overwhelmed":
	default:
		return deny()
	}
	feeling := experience.Feeling
	text := ""
	switch d.Mode {
	case "full":
		degree := "a little"
		if experience.Intensity >= .7 {
			degree = "very"
		} else if experience.Intensity >= .3 {
			degree = "moderately"
		}
		text = fmt.Sprintf("I feel %s %s.", degree, feeling)
	case "partial":
		if feeling == "grateful" {
			text = "Something has meant a lot to me."
		} else {
			text = "I've been having a difficult time."
		}
	case "softened":
		text = "I feel a little " + feeling + ", though I'm not ready to say more."
	case "joke":
		text = "My feeling-" + feeling + " meter is working overtime."
	case "deflection":
		text = "I'd rather talk about something else."
	case "lie":
		text = "I do not feel " + feeling + "." // fictional behavior only, never AssistantDisclosure
	case "omission":
		text = "There is more to this, but I am keeping part private."
	case "topic_change":
		text = "Let's change the topic."
	case "silence":
		text = ""
	default:
		return deny()
	}
	evidence := item
	evidence.Text = text
	return ValidatedOutput{Text: text, Sources: []core.ID{item.Source}, Evidence: []SafeContextItem{evidence}, FictionMode: d.Mode}, PolicyDecision{Allowed: true, Action: "ALLOW"}, nil
}
