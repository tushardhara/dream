package postgres

import (
	"context"
	"fmt"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// Revoke appends a versioned event and atomically tombstones/purges its source
// and all descendants. The new audit envelope remains, but its reason payload
// is also purged because it derives from the revoked source.
func (s *Store) Revoke(ctx context.Context, c graph.AppendCommand, root core.ID) (graph.AppendResult, error) {
	if err := root.Validate(); err != nil {
		return graph.AppendResult{}, err
	}
	if c.Event.Type != "revoke" {
		return graph.AppendResult{}, fmt.Errorf("revocation event required")
	}
	c.Event.Meta.Parents = append(append([]core.ID{}, c.Event.Meta.Parents...), root)
	tx, err := s.begin(ctx)
	if err != nil {
		return graph.AppendResult{}, err
	}
	defer tx.Rollback(ctx)
	result, err := s.append(ctx, tx, c)
	if err != nil {
		return result, err
	}
	_, err = tx.Exec(ctx, `WITH RECURSIVE affected(id) AS(SELECT $3::text UNION SELECT l.child FROM dream.lineage l JOIN affected a ON l.parent=a.id WHERE l.actor=$1 AND l.namespace=$2) INSERT INTO dream.tombstones SELECT $1,$2,id,clock_timestamp() FROM affected ON CONFLICT DO NOTHING`, c.Actor, c.Namespace, root)
	if err != nil {
		return result, err
	}
	// Pending injected input is restricted too; purge operation requests for
	// revoked runs alongside checkpoints so restart cannot resurrect them.
	if _, err = tx.Exec(ctx, `DELETE FROM dream.runtime_operations o USING dream.runtime_heads h,dream.tombstones t WHERE (o.actor,o.namespace,o.run)=(h.actor,h.namespace,h.run) AND (h.actor,h.namespace,h.event_id)=(t.actor,t.namespace,t.event_id) AND o.actor=$1 AND o.namespace=$2`, c.Actor, c.Namespace); err != nil {
		return result, err
	}
	for _, table := range []string{"observable_payloads", "private_payloads", "research_payloads", "runtime_payloads", "projections"} {
		if _, err = tx.Exec(ctx, `DELETE FROM dream.`+table+` p USING dream.tombstones t WHERE(p.actor,p.namespace,p.event_id)=(t.actor,t.namespace,t.event_id) AND t.actor=$1 AND t.namespace=$2`, c.Actor, c.Namespace); err != nil {
			return result, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE dream.artifact_refs a SET invalidated=true WHERE actor=$1 AND namespace=$2 AND EXISTS(SELECT 1 FROM dream.artifact_sources src JOIN dream.tombstones t USING(actor,namespace,event_id) WHERE(src.actor,src.namespace,src.artifact_id)=(a.actor,a.namespace,a.id))`, c.Actor, c.Namespace); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

// RegisterArtifact records references only, not a snapshot/export implementation.
// Referenced source events initiate these derived records. No payload is copied.
func (s *Store) RegisterArtifact(ctx context.Context, actor, namespace, id core.ID, kind string, sources []core.ID) error {
	for _, v := range append([]core.ID{actor, namespace, id}, sources...) {
		if err := v.Validate(); err != nil {
			return err
		}
	}
	if (kind != "snapshot" && kind != "export") || len(sources) == 0 {
		return fmt.Errorf("invalid artifact")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, source := range sources {
		var revoked bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dream.tombstones WHERE actor=$1 AND namespace=$2 AND event_id=$3)`, actor, namespace, source).Scan(&revoked); err != nil {
			return err
		}
		if revoked {
			return ErrRevoked
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dream.artifact_refs(actor,namespace,id,kind) VALUES($1,$2,$3,$4)`, actor, namespace, id, kind); err != nil {
		return err
	}
	for _, source := range sources {
		if _, err = tx.Exec(ctx, `INSERT INTO dream.artifact_sources VALUES($1,$2,$3,$4)`, actor, namespace, id, source); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) ArtifactValid(ctx context.Context, actor, namespace, id core.ID) (bool, error) {
	var valid bool
	err := s.db.QueryRow(ctx, `SELECT NOT invalidated AND NOT EXISTS(SELECT 1 FROM dream.artifact_sources src JOIN dream.tombstones t USING(actor,namespace,event_id) WHERE(src.actor,src.namespace,src.artifact_id)=(a.actor,a.namespace,a.id)) FROM dream.artifact_refs a WHERE actor=$1 AND namespace=$2 AND id=$3`, actor, namespace, id).Scan(&valid)
	return valid, err
}
