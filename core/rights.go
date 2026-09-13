package core

import "fmt"

type Operation string
const (
 Read Operation="read"
 Derive Operation="derive"
 Disclose Operation="disclose"
 Attribute Operation="attribute"
 Aggregate Operation="aggregate"
 Match Operation="match"
 Retain Operation="retain"
 Export Operation="export"
 ShareOnRequest Operation="share_on_request"
)
func (o Operation) valid()bool {switch o {case Read,Derive,Disclose,Attribute,Aggregate,Match,Retain,Export,ShareOnRequest:return true};return false}

// Grant is one exact context, never a wildcard. Recipient is explicit even for
// read/derive (use the acting principal for self-use). Purpose is a logical ID.
// Authentication, consent provenance, expiry and revocation lookup belong to
// consuming policy services; these values alone are not trusted credentials.
type Grant struct { Actor ID `json:"actor"`; Recipient ID `json:"recipient"`; Purpose ID `json:"purpose"`; Operation Operation `json:"operation"` }
func (g Grant) Validate()error{if err:=ids(g.Actor,g.Recipient,g.Purpose);err!=nil{return err};if !g.Operation.valid(){return fmt.Errorf("unknown operation")};return nil}
type PermissionRequest struct { Resource ID `json:"resource"`; Context Grant `json:"context"` }
type Rights struct { Resource ID `json:"resource"`; Revoked bool `json:"revoked"`; Grants []Grant `json:"grants"` }
func (r Rights) Validate()error {if err:=r.Resource.Validate();err!=nil{return err};seen:=map[Grant]bool{};for _,g:=range r.Grants {if err:=g.Validate();err!=nil{return err};if seen[g]{return fmt.Errorf("duplicate grant")};seen[g]=true};return nil}
func (r Rights) Allows(q PermissionRequest)bool {
 if r.Validate()!=nil||q.Resource.Validate()!=nil||q.Context.Validate()!=nil||r.Revoked||q.Resource!=r.Resource{return false}
 for _,g:=range r.Grants {if g==q.Context{return true}};return false
}

// DeriveRights requires derive permission on EVERY supplied source and returns
// only their grant intersection. The caller must supply the complete, currently
// authorized lineage; #12's policy service must resolve it from trusted storage.
// This contract never invents new disclosure/attribution/export rights.
func DeriveRights(resource ID, context Grant, sources []Rights)(Rights,error){
 out:=Rights{Resource:resource,Grants:[]Grant{}}
 if err:=resource.Validate();err!=nil{return Rights{},err}
 if context.Operation!=Derive||context.Validate()!=nil||len(sources)==0{return Rights{},fmt.Errorf("known derive context and sources required")}
 seen:=map[ID]bool{}
 for _,source:=range sources {
  if source.Resource==resource||seen[source.Resource]||!source.Allows(PermissionRequest{source.Resource,context}){return Rights{},fmt.Errorf("invalid or unauthorized source")};seen[source.Resource]=true
 }
 for _,g:=range sources[0].Grants {
  allowed:=true
  for _,source:=range sources[1:] {if !source.Allows(PermissionRequest{source.Resource,g}){allowed=false;break}}
  if allowed {out.Grants=append(out.Grants,g)}
 }
 sortGrants(out.Grants)
 return out,nil
}
