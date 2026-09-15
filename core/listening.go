package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const ListeningVersion = "listening-account.v1"

// ListeningClause is what Speaker reports, values, or hypothesizes. Even an
// observation is an attributed report, never a finding that another person lied.
type ListeningClause struct {
	Key        ID
	Kind       string // observation, value, hypothesis
	About      ID
	Text       string
	Confidence Confidence
}

// CommunicationPreferences are participant choices, not demographic proxies.
// Language tags outside the tested renderer scope remain valid data; the host
// must WAIT rather than silently translate them or infer a cultural preference.
type CommunicationPreferences struct {
	Language      ID
	Style         string // unspecified, literal, indirect
	Channel       string // text, transcript
	ShortTurns    bool
	PlainLanguage bool
}

// ListeningAccount is stored inside an observer-owned memory event. The host
// must bind Source/Speaker/Corrects to that event's current authorship and lineage.
// Mixed feelings and alternative hypotheses coexist without being averaged.
type ListeningAccount struct {
	Version      string
	Source       ID
	Speaker      ID
	Other        ID
	Focus        RelationshipFocus
	Original     string
	Clauses      []ListeningClause
	Feelings     []string
	DesiredHelp  string // unknown, listen, understand, coordinate, pause
	Preferences  CommunicationPreferences
	Confirmation string // unconfirmed, confirmed, corrected
	Corrects     ID     `json:",omitempty"`
	Summary      ID     `json:",omitempty"` // separately authored, explicitly shareable words
}

func listeningText(s string, max int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && len(s) <= max
}

func (a ListeningAccount) Validate() error {
	if a.Version != ListeningVersion || ids(a.Source, a.Speaker, a.Other) != nil || a.Speaker == a.Other || a.Focus.Validate() != nil || a.Focus.RoleContext == "" || !listeningText(a.Original, 512) || len(a.Clauses) > 6 || len(a.Feelings) > 4 {
		return fmt.Errorf("invalid listening account")
	}
	switch a.DesiredHelp {
	case "unknown", "listen", "understand", "coordinate", "pause":
	default:
		return fmt.Errorf("unknown desired help")
	}
	switch a.Confirmation {
	case "unconfirmed", "confirmed":
		if a.Corrects != "" {
			return fmt.Errorf("unexpected listening correction")
		}
	case "corrected":
		if a.Corrects.Validate() != nil || a.Corrects == a.Source {
			return fmt.Errorf("invalid listening correction")
		}
	default:
		return fmt.Errorf("unknown listening confirmation")
	}
	p := a.Preferences
	if p.Language.Validate() != nil || p.Style != "unspecified" && p.Style != "literal" && p.Style != "indirect" || p.Channel != "text" && p.Channel != "transcript" {
		return fmt.Errorf("invalid declared communication preferences")
	}
	if a.Summary != "" && (a.Summary.Validate() != nil || a.Summary == a.Source || a.Confirmation == "unconfirmed") {
		return fmt.Errorf("unconfirmed or invalid summary selection")
	}
	seen := map[ID]bool{}
	for _, c := range a.Clauses {
		if c.Key.Validate() != nil || seen[c.Key] || c.About != a.Speaker && c.About != a.Other || !listeningText(c.Text, 160) || c.Confidence.Validate() != nil {
			return fmt.Errorf("invalid attributed listening clause")
		}
		seen[c.Key] = true
		switch c.Kind {
		case "observation", "value":
		case "hypothesis":
			if c.Confidence >= 1 {
				return fmt.Errorf("hypothesis asserts certainty")
			}
		default:
			return fmt.Errorf("unknown listening clause kind")
		}
	}
	feelings := map[string]bool{}
	for _, feeling := range a.Feelings {
		if !listeningText(feeling, 64) || feelings[feeling] {
			return fmt.Errorf("invalid declared feeling")
		}
		feelings[feeling] = true
	}
	raw, err := json.Marshal(a)
	if err != nil || len(raw) > 2048 {
		return fmt.Errorf("listening account bound")
	}
	return nil
}

func EncodeListeningAccount(a ListeningAccount) ([]byte, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func DecodeListeningAccount(raw []byte) (ListeningAccount, error) {
	var a ListeningAccount
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 2048 || d.Decode(&a) != nil || d.Decode(new(any)) != io.EOF || a.Validate() != nil {
		return ListeningAccount{}, fmt.Errorf("invalid listening encoding")
	}
	return a, nil
}
