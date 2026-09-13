package core

import (
 "fmt"
)

// State is a closed validation/serialization bundle, not a database or event
// store. All referenced principals, evidence and lineage must be included.
// It has no world/run/branch. Partial retrieval gets its own contract in #8.
type State struct {
 Version uint32 `json:"version"`
 Principals []Principal `json:"principals"`
 References []NonparticipantReference `json:"references"`
 Evidence []Evidence `json:"evidence"`
 Events []Event `json:"events"`
 Claims []Claim `json:"claims"`
 Hypotheses []Hypothesis `json:"hypotheses"`
 Relationships []Relationship `json:"relationships"`
 Groups []Group `json:"groups"`
 Memories []Memory `json:"memories"`
 OpenLoops []OpenLoop `json:"openloops"`
 Intents []Intent `json:"intents"`
 Proposals []Proposal `json:"proposals"`
 Decisions []Decision `json:"decisions"`
 Outcomes []Outcome `json:"outcomes"`
}

func (s State) Validate()error {
 if s.Version!=1{return fmt.Errorf("unsupported state version")}
 count:=len(s.Principals)+len(s.References)
 count+=len(s.Evidence)
 count+=len(s.Events)
 count+=len(s.Claims)
 count+=len(s.Hypotheses)
 count+=len(s.Relationships)
 count+=len(s.Groups)
 count+=len(s.Memories)
 count+=len(s.OpenLoops)
 count+=len(s.Intents)
 count+=len(s.Proposals)
 count+=len(s.Decisions)
 count+=len(s.Outcomes)
 if count>10000{return fmt.Errorf("state exceeds 10000 record validation cap")}
 principals:=map[ID]bool{}
 for _,p:=range s.Principals {if err:=p.Validate();err!=nil{return err};if principals[p.ID]{return fmt.Errorf("duplicate principal")};principals[p.ID]=true}
 refs:=map[NonparticipantReference]bool{}
 for _,r:=range s.References {if err:=r.Validate();err!=nil{return err};if !principals[r.Observer]||refs[r]{return fmt.Errorf("unknown observer or duplicate reference")};refs[r]=true}
 records:=map[ID]Metadata{}
 add:=func(m Metadata)error{if _,ok:=records[m.ID];ok{return fmt.Errorf("duplicate record ID %s",m.ID)};if !principals[m.Observer]||!principals[m.Source]{return fmt.Errorf("unknown observer/source")};records[m.ID]=m;return nil}
 for _,v:=range s.Evidence {if err:=v.Validate();err!=nil{return fmt.Errorf("Evidence: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Events {if err:=v.Validate();err!=nil{return fmt.Errorf("Events: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Claims {if err:=v.Validate();err!=nil{return fmt.Errorf("Claims: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Hypotheses {if err:=v.Validate();err!=nil{return fmt.Errorf("Hypotheses: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Relationships {if err:=v.Validate();err!=nil{return fmt.Errorf("Relationships: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Groups {if err:=v.Validate();err!=nil{return fmt.Errorf("Groups: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Memories {if err:=v.Validate();err!=nil{return fmt.Errorf("Memories: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.OpenLoops {if err:=v.Validate();err!=nil{return fmt.Errorf("OpenLoops: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Intents {if err:=v.Validate();err!=nil{return fmt.Errorf("Intents: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Proposals {if err:=v.Validate();err!=nil{return fmt.Errorf("Proposals: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Decisions {if err:=v.Validate();err!=nil{return fmt.Errorf("Decisions: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 for _,v:=range s.Outcomes {if err:=v.Validate();err!=nil{return fmt.Errorf("Outcomes: %w",err)};if err:=add(v.Meta);err!=nil{return err}}
 evidence:=map[ID]bool{};for _,v:=range s.Evidence {evidence[v.Meta.ID]=true}
 for _,m:=range records {
  for _,list:=range [][]ID{m.Supporting,m.Contradicting} {for _,id:=range list {if !evidence[id]{return fmt.Errorf("missing evidence %s",id)}}}
  for _,id:=range m.Parents {
   parent,ok:=records[id];if !ok{return fmt.Errorf("missing lineage %s",id)}
   if parent.Sensitivity==Restricted&&m.Sensitivity!=Restricted{return fmt.Errorf("derived sensitivity broadened")}
   if !m.Rights.Revoked {if parent.Rights.Revoked{return fmt.Errorf("revoked lineage")};for _,g:=range m.Rights.Grants {if !parent.Rights.Allows(PermissionRequest{parent.ID,g}){return fmt.Errorf("derived rights broadened")}}}
  }
 }
 // Cycle detection traverses lineage and evidence dependencies, not just self IDs.
 colors:=map[ID]uint8{}
 var visit func(ID)error
 visit=func(id ID)error{if colors[id]==1{return fmt.Errorf("cyclic lineage")};if colors[id]==2{return nil};colors[id]=1;m:=records[id];for _,list:=range [][]ID{m.Parents,m.Supporting,m.Contradicting}{for _,p:=range list {if err:=visit(p);err!=nil{return err}}};colors[id]=2;return nil}
 for id:=range records {if err:=visit(id);err!=nil{return err}}
 subject:=func(v Subject)error{if v.Reference!=nil {if !refs[*v.Reference]{return fmt.Errorf("unknown observer-owned reference")}}else if !principals[v.Principal]{return fmt.Errorf("unknown principal subject")};return nil}
 for _,v:=range s.Events {if err:=subject(v.Subject);err!=nil{return err}}
 for _,v:=range s.Claims {if err:=subject(v.Subject);err!=nil{return err}}
 for _,v:=range s.Hypotheses {if err:=subject(v.Subject);err!=nil{return err}}
 for _,v:=range s.Relationships {if err:=subject(v.From);err!=nil{return err};if err:=subject(v.To);err!=nil{return err}}
 for _,v:=range s.Groups {for _,m:=range v.Members {if err:=subject(m);err!=nil{return err}}}
 intents:=map[ID]Intent{};for _,v:=range s.Intents {if !principals[v.Actor]{return fmt.Errorf("unknown intent actor")};intents[v.Meta.ID]=v}
 proposals:=map[ID]Proposal{};for _,v:=range s.Proposals {if _,ok:=intents[v.Intent];!ok{return fmt.Errorf("missing intent")};proposals[v.Meta.ID]=v}
 decisions:=map[ID]bool{};for _,v:=range s.Decisions {if !principals[v.Actor]{return fmt.Errorf("unknown decision actor")};if v.Kind==Act {p,ok:=proposals[v.Proposal];if !ok||intents[p.Intent].Actor!=v.Actor{return fmt.Errorf("invalid decision proposal/actor")}};decisions[v.Meta.ID]=true}
 for _,v:=range s.Outcomes {if !decisions[v.Decision]||!principals[v.AffectedObserver]{return fmt.Errorf("unknown outcome decision/observer")}}
 return nil
}
