-- Run only with migration authority in a dedicated Dream database/cluster.
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='dream_writer') THEN CREATE ROLE dream_writer NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS; ELSIF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='dream_writer' AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolcanlogin)) THEN RAISE EXCEPTION 'unsafe existing role dream_writer'; END IF; END $$;
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='dream_actor') THEN CREATE ROLE dream_actor NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS; ELSIF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='dream_actor' AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolcanlogin)) THEN RAISE EXCEPTION 'unsafe existing role dream_actor'; END IF; END $$;
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='dream_private_reader') THEN CREATE ROLE dream_private_reader NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS; ELSIF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='dream_private_reader' AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolcanlogin)) THEN RAISE EXCEPTION 'unsafe existing role dream_private_reader'; END IF; END $$;
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='dream_research_reader') THEN CREATE ROLE dream_research_reader NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS; ELSIF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='dream_research_reader' AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolcanlogin)) THEN RAISE EXCEPTION 'unsafe existing role dream_research_reader'; END IF; END $$;
CREATE SCHEMA dream;
REVOKE ALL ON SCHEMA dream FROM PUBLIC;
GRANT USAGE ON SCHEMA dream TO dream_writer,dream_actor,dream_private_reader,dream_research_reader;
CREATE TABLE dream.schema_versions(version integer PRIMARY KEY CHECK(version>0));
INSERT INTO dream.schema_versions VALUES(1);
CREATE TABLE dream.streams(actor text NOT NULL, namespace text NOT NULL, stream text NOT NULL, version bigint NOT NULL CHECK(version>=0), PRIMARY KEY(actor,namespace,stream));
CREATE TABLE dream.events(
 seq bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
 actor text NOT NULL, namespace text NOT NULL, id text NOT NULL, stream text NOT NULL,
 stream_version bigint NOT NULL CHECK(stream_version>0), schema_version integer NOT NULL CHECK(schema_version=1),
 subject text NOT NULL, occurred_at bigint NOT NULL CHECK(occurred_at>=0),
 valid_from bigint NOT NULL CHECK(valid_from>=0), valid_to bigint CHECK(valid_to>valid_from),
 recorded_at timestamptz NOT NULL, envelope jsonb NOT NULL,
 payload_class text NOT NULL CHECK(payload_class IN ('observable','private','research')),
 supersedes text,
 PRIMARY KEY(actor,namespace,id), UNIQUE(actor,namespace,stream,stream_version),
 FOREIGN KEY(actor,namespace,stream) REFERENCES dream.streams,
 FOREIGN KEY(actor,namespace,supersedes) REFERENCES dream.events(actor,namespace,id)
);
CREATE TABLE dream.observable_payloads(actor text NOT NULL, namespace text NOT NULL, event_id text NOT NULL, payload bytea NOT NULL, PRIMARY KEY(actor,namespace,event_id), FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id));
CREATE TABLE dream.private_payloads(LIKE dream.observable_payloads INCLUDING ALL);
ALTER TABLE dream.private_payloads ADD FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id);
CREATE TABLE dream.research_payloads(LIKE dream.observable_payloads INCLUDING ALL);
ALTER TABLE dream.research_payloads ADD FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id);
-- A generic actor role alone has no rows; authenticated login identity scopes
-- observable access. Never use a user-settable role header/GUC as identity.
ALTER TABLE dream.observable_payloads ENABLE ROW LEVEL SECURITY;
CREATE POLICY observable_actor ON dream.observable_payloads TO dream_actor USING(actor=session_user);
CREATE POLICY observable_writer ON dream.observable_payloads TO dream_writer USING(true) WITH CHECK(true);
CREATE TABLE dream.lineage(actor text NOT NULL, namespace text NOT NULL, child text NOT NULL, parent text NOT NULL, PRIMARY KEY(actor,namespace,child,parent), CHECK(child<>parent), FOREIGN KEY(actor,namespace,child) REFERENCES dream.events(actor,namespace,id), FOREIGN KEY(actor,namespace,parent) REFERENCES dream.events(actor,namespace,id));
CREATE TABLE dream.tombstones(actor text NOT NULL, namespace text NOT NULL, event_id text NOT NULL, revoked_at timestamptz NOT NULL, PRIMARY KEY(actor,namespace,event_id), FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id));
CREATE TABLE dream.command_results(actor text NOT NULL, namespace text NOT NULL, operation text NOT NULL, key text NOT NULL, digest bytea NOT NULL, event_id text NOT NULL, seq bigint NOT NULL, stream_version bigint NOT NULL, PRIMARY KEY(actor,namespace,operation,key), FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id));
CREATE TABLE dream.outbox(actor text NOT NULL, namespace text NOT NULL, event_id text NOT NULL, delivered boolean NOT NULL DEFAULT false, PRIMARY KEY(actor,namespace,event_id), FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id));
CREATE TABLE dream.projections(actor text NOT NULL, namespace text NOT NULL, event_id text NOT NULL, seq bigint NOT NULL, subject text NOT NULL, valid_from bigint NOT NULL, valid_to bigint, recorded_at timestamptz NOT NULL, PRIMARY KEY(actor,namespace,event_id), FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id));
CREATE TABLE dream.projection_checkpoints(name text PRIMARY KEY, seq bigint NOT NULL CHECK(seq>=0));
CREATE TABLE dream.jobs(id text PRIMARY KEY, actor text NOT NULL, namespace text NOT NULL, status text NOT NULL CHECK(status IN ('pending','running','done')), checkpoint_ref text);
CREATE TABLE dream.artifact_refs(actor text NOT NULL, namespace text NOT NULL, id text NOT NULL, kind text NOT NULL CHECK(kind IN ('snapshot','export')), invalidated boolean NOT NULL DEFAULT false, PRIMARY KEY(actor,namespace,id));
CREATE TABLE dream.artifact_sources(actor text NOT NULL, namespace text NOT NULL, artifact_id text NOT NULL, event_id text NOT NULL, PRIMARY KEY(actor,namespace,artifact_id,event_id), FOREIGN KEY(actor,namespace,artifact_id) REFERENCES dream.artifact_refs(actor,namespace,id), FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id));
CREATE TABLE dream.simulator_associations(actor text NOT NULL, namespace text NOT NULL, event_id text NOT NULL, run_id text NOT NULL, branch_id text NOT NULL, learner text NOT NULL, learned_at bigint CHECK(learned_at>=0), PRIMARY KEY(actor,namespace,event_id,run_id,branch_id,learner), FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id));
REVOKE ALL ON ALL TABLES IN SCHEMA dream FROM PUBLIC,dream_actor,dream_private_reader,dream_research_reader;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA dream TO dream_writer;
REVOKE UPDATE,DELETE ON dream.events,dream.tombstones,dream.lineage FROM dream_writer;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA dream TO dream_writer;
GRANT SELECT ON dream.observable_payloads TO dream_actor;
GRANT SELECT ON dream.private_payloads TO dream_private_reader;
GRANT SELECT ON dream.research_payloads TO dream_research_reader;
REVOKE INSERT,UPDATE,DELETE ON dream.schema_versions FROM dream_writer;
