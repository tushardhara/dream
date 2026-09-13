-- Restricted simulator state is associated with generic journal events. No actor
-- or reader role receives these research/operational tables implicitly.
CREATE TABLE dream.runtime_heads(
 actor text NOT NULL, namespace text NOT NULL, world text NOT NULL, branch text NOT NULL, run text NOT NULL,
 event_id text NOT NULL, revision bigint NOT NULL CHECK(revision>0), manifest_hash text NOT NULL,
 deadline timestamptz NOT NULL, holder text, fence bigint NOT NULL DEFAULT 0 CHECK(fence>=0), expires timestamptz,
 PRIMARY KEY(actor,namespace,world,branch), UNIQUE(actor,namespace,run),
 FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id)
);
CREATE TABLE dream.runtime_payloads(actor text NOT NULL, namespace text NOT NULL,event_id text NOT NULL,payload bytea NOT NULL,
 PRIMARY KEY(actor,namespace,event_id),FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id));
CREATE TABLE dream.runtime_operations(actor text NOT NULL,namespace text NOT NULL,run text NOT NULL,key text NOT NULL,
 request bytea NOT NULL,digest text NOT NULL,receipt bytea NOT NULL,done boolean NOT NULL,
 PRIMARY KEY(actor,namespace,run,key),FOREIGN KEY(actor,namespace,run) REFERENCES dream.runtime_heads(actor,namespace,run));
CREATE UNIQUE INDEX runtime_one_operation ON dream.runtime_operations(actor,namespace,run) WHERE NOT done;
CREATE TABLE dream.runtime_lease_audit(actor text NOT NULL,namespace text NOT NULL,run text NOT NULL,
 fence bigint NOT NULL,holder text NOT NULL,recorded_at timestamptz NOT NULL,expires timestamptz NOT NULL,version int NOT NULL CHECK(version=1));
REVOKE ALL ON dream.runtime_heads,dream.runtime_payloads,dream.runtime_operations,dream.runtime_lease_audit FROM PUBLIC,dream_actor,dream_private_reader,dream_research_reader;
GRANT SELECT,INSERT,UPDATE ON dream.runtime_heads,dream.runtime_operations TO dream_writer;
GRANT SELECT,INSERT,DELETE ON dream.runtime_payloads TO dream_writer;
GRANT DELETE ON dream.runtime_operations TO dream_writer;
GRANT SELECT,INSERT ON dream.runtime_lease_audit TO dream_writer;
INSERT INTO dream.schema_versions VALUES(2);
