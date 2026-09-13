package core

import (
 "fmt"
 "math"
 "strings"
 "time"
 "unicode/utf8"
)

// ID is a logical identifier, never an inferred real-world identity. IDs are
// case-sensitive ASCII tokens of 1..128 bytes; callers inject them.
type ID string
func NewID(s string) (ID,error) { id:=ID(s); return id,id.Validate() }
func (id ID) Validate() error {
 if len(id)==0 || len(id)>128 { return fmt.Errorf("invalid ID length") }
 for _,c:=range id { if !(c>='a'&&c<='z'||c>='A'&&c<='Z'||c>='0'&&c<='9'||c=='-'||c=='_'||c=='.'||c==':') { return fmt.Errorf("invalid ID character") } }
 return nil
}
func ids(values ...ID) error { for _,v:=range values {if err:=v.Validate();err!=nil{return err}};return nil }
func unique(values []ID) error {seen:=map[ID]bool{};for _,id:=range values {if err:=id.Validate();err!=nil{return err};if seen[id]{return fmt.Errorf("duplicate ID %s",id)};seen[id]=true};return nil}
func text(s string) error {if !utf8.ValidString(s)||strings.TrimSpace(s)==""||len(s)>4096{return fmt.Errorf("invalid text")};return nil}

// LogicalTime is caller-supplied nonnegative nanoseconds from a host-defined
// epoch. It is not a wall clock or an actor's learned-at time.
type LogicalTime int64
func (v LogicalTime) Validate() error {if v<0{return fmt.Errorf("negative logical time")};return nil}
type Interval struct { Start LogicalTime `json:"start"`; End *LogicalTime `json:"end,omitempty"` }
func (v Interval) Validate() error {if err:=v.Start.Validate();err!=nil{return err};if v.End!=nil&&*v.End<=v.Start{return fmt.Errorf("interval must be nonempty [start,end)")};return nil}
func recorded(t time.Time) error {if t.IsZero()||t.Year()<1||t.Year()>9999{return fmt.Errorf("invalid recorded system time")};return nil}

// Confidence is a bounded subjective strength, NOT a calibrated probability.
type Confidence float64
func (c Confidence) Validate() error {return unit(float64(c))}
func unit(v float64) error {if math.IsNaN(v)||math.IsInf(v,0)||v<0||v>1{return fmt.Errorf("expected finite value in [0,1]")};return nil}
type CalibratedProbability struct { Value float64 `json:"value"`; Calibration ID `json:"calibration"` }
func (p CalibratedProbability) Validate() error {if err:=unit(p.Value);err!=nil{return err};return p.Calibration.Validate()}

type Principal struct { ID ID `json:"id"` }
func NewPrincipal(id ID)(Principal,error){p:=Principal{id};return p,p.Validate()}
func (p Principal) Validate()error{return p.ID.Validate()}

// NonparticipantReference is owned by one observer. The same local ID under a
// different observer is a distinct identity; no global person is implied.
type NonparticipantReference struct { Observer ID `json:"observer"`; LocalID ID `json:"local_id"` }
func NewReference(observer,local ID)(NonparticipantReference,error){r:=NonparticipantReference{observer,local};return r,r.Validate()}
func (r NonparticipantReference) Validate()error{return ids(r.Observer,r.LocalID)}
type Subject struct { Principal ID `json:"principal,omitempty"`; Reference *NonparticipantReference `json:"reference,omitempty"` }
func (s Subject) ValidateFor(observer ID)error{
 if err:=observer.Validate();err!=nil{return err}
 if (s.Principal=="")== (s.Reference==nil){return fmt.Errorf("subject must be exactly one principal or reference")}
 if s.Reference!=nil {if err:=s.Reference.Validate();err!=nil{return err};if s.Reference.Observer!=observer{return fmt.Errorf("foreign observer reference")};return nil};return s.Principal.Validate()
}
