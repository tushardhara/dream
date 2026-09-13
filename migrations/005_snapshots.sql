-- Snapshot bodies are restricted internal artifacts. A key is never authority.
CREATE TABLE dream.snapshot_heads(
 actor text NOT NULL,namespace text NOT NULL,world text NOT NULL,branch text NOT NULL,run text NOT NULL,
 id text NOT NULL,event_id text NOT NULL,hash text NOT NULL CHECK(length(hash)=64),revision bigint NOT NULL CHECK(revision>0),
 PRIMARY KEY(actor,namespace,run,id),
 FOREIGN KEY(actor,namespace,run) REFERENCES dream.runtime_heads(actor,namespace,run),
 FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id)
);
CREATE TABLE dream.snapshot_payloads(
 actor text NOT NULL,namespace text NOT NULL,event_id text NOT NULL,payload bytea NOT NULL CHECK(octet_length(payload)<=8388608),
 PRIMARY KEY(actor,namespace,event_id),FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id)
);
ALTER TABLE dream.snapshot_payloads ENABLE ROW LEVEL SECURITY;
ALTER TABLE dream.snapshot_payloads FORCE ROW LEVEL SECURITY;
CREATE POLICY snapshot_writer ON dream.snapshot_payloads TO dream_writer USING(true) WITH CHECK(true);
CREATE TABLE dream.derived_sources(
 actor text NOT NULL,namespace text NOT NULL,event_id text NOT NULL,
 source_actor text NOT NULL,source_namespace text NOT NULL,source_event text NOT NULL,
 PRIMARY KEY(actor,namespace,event_id,source_actor,source_namespace,source_event),
 FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id),
 FOREIGN KEY(source_actor,source_namespace,source_event) REFERENCES dream.events(actor,namespace,id)
);
CREATE INDEX derived_source_lookup ON dream.derived_sources(source_actor,source_namespace,source_event);
CREATE TABLE dream.branch_origins(
 actor text NOT NULL,namespace text NOT NULL,run text NOT NULL,event_id text NOT NULL,
 snapshot_event text NOT NULL,request_hash text NOT NULL,mode text NOT NULL CHECK(mode='fresh_resimulation'),coupling text NOT NULL CHECK(coupling IN ('common-exogenous-counter.v1','independent-branch-streams.v1')),
 PRIMARY KEY(actor,namespace,run),FOREIGN KEY(actor,namespace,run) REFERENCES dream.runtime_heads(actor,namespace,run),
 FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id),
 FOREIGN KEY(actor,namespace,snapshot_event) REFERENCES dream.events(actor,namespace,id)
);
REVOKE ALL ON dream.snapshot_heads,dream.snapshot_payloads,dream.derived_sources,dream.branch_origins FROM PUBLIC,dream_actor,dream_private_reader,dream_research_reader;
GRANT SELECT,INSERT ON dream.snapshot_heads,dream.derived_sources,dream.branch_origins TO dream_writer;
GRANT SELECT,INSERT,DELETE ON dream.snapshot_payloads TO dream_writer;
INSERT INTO dream.schema_versions VALUES(5);
