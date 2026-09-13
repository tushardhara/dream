-- Durable request/attempt metadata is separate from purgeable model artifacts.
CREATE TABLE dream.model_budgets(
 actor text NOT NULL,namespace text NOT NULL,run text NOT NULL,
 limits bytea NOT NULL,tokens bigint NOT NULL DEFAULT 0,spend bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(actor,namespace,run),
 FOREIGN KEY(actor,namespace,run) REFERENCES dream.runtime_heads(actor,namespace,run)
);
CREATE TABLE dream.model_requests(
 actor text NOT NULL,namespace text NOT NULL,run text NOT NULL,key text NOT NULL,
 principal text NOT NULL,memory_namespace text NOT NULL,event_id text NOT NULL,
 digest text NOT NULL,status text NOT NULL,attempt int NOT NULL,fence bigint NOT NULL,
 expires timestamptz NOT NULL,event_version bigint NOT NULL,
 PRIMARY KEY(actor,namespace,run,key),
 FOREIGN KEY(actor,namespace,run) REFERENCES dream.model_budgets(actor,namespace,run),
 FOREIGN KEY(principal,memory_namespace,event_id) REFERENCES dream.events(actor,namespace,id)
);
CREATE TABLE dream.model_payloads(
 actor text NOT NULL,namespace text NOT NULL,event_id text NOT NULL,payload bytea NOT NULL,
 PRIMARY KEY(actor,namespace,event_id),
 FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id)
);
CREATE TABLE dream.model_applications(
 actor text NOT NULL,namespace text NOT NULL,run text NOT NULL,key text NOT NULL,
 event_id text NOT NULL,
 PRIMARY KEY(actor,namespace,run,key),
 FOREIGN KEY(actor,namespace,run,key) REFERENCES dream.model_requests(actor,namespace,run,key),
 FOREIGN KEY(actor,namespace,event_id) REFERENCES dream.events(actor,namespace,id)
);
REVOKE ALL ON dream.model_budgets,dream.model_requests,dream.model_payloads,dream.model_applications FROM PUBLIC,dream_actor,dream_private_reader,dream_research_reader;
GRANT SELECT,INSERT,UPDATE ON dream.model_budgets,dream.model_requests TO dream_writer;
GRANT SELECT,INSERT,DELETE ON dream.model_payloads TO dream_writer;
GRANT SELECT,INSERT ON dream.model_applications TO dream_writer;
ALTER TABLE dream.model_payloads ENABLE ROW LEVEL SECURITY;
ALTER TABLE dream.model_payloads FORCE ROW LEVEL SECURITY;
CREATE POLICY model_writer ON dream.model_payloads TO dream_writer USING(true) WITH CHECK(true);
INSERT INTO dream.schema_versions VALUES(4);
