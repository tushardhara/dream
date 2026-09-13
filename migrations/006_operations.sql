-- Append-only settlement facts supplement (never refund) conservative reservations.
-- No prompt, output, source text or credentials. The event FK binds each fact to
-- the versioned attempt event committed in the same transaction.
CREATE TABLE dream.model_usage (
 actor text NOT NULL, namespace text NOT NULL, run text NOT NULL, key text NOT NULL,
 attempt integer NOT NULL CHECK(attempt>0),
 principal text NOT NULL, memory_namespace text NOT NULL, event_id text NOT NULL,
 status text NOT NULL CHECK(status IN ('complete','refused','malformed','rate_limited','unavailable')),
 input_tokens bigint, output_tokens bigint,
 CHECK ((input_tokens IS NULL AND output_tokens IS NULL) OR
        (input_tokens>=0 AND output_tokens>=0 AND input_tokens IS NOT NULL AND output_tokens IS NOT NULL)),
 PRIMARY KEY(actor,namespace,run,key,attempt),
 FOREIGN KEY(actor,namespace,run,key) REFERENCES dream.model_requests(actor,namespace,run,key),
 FOREIGN KEY(principal,memory_namespace,event_id) REFERENCES dream.events(actor,namespace,id)
);
REVOKE ALL ON dream.model_usage FROM PUBLIC,dream_actor,dream_private_reader,dream_research_reader;
GRANT SELECT,INSERT ON dream.model_usage TO dream_writer;
INSERT INTO dream.schema_versions VALUES(6);
-- Restore admission is explicitly closed before applying an externally retained
-- revocation journal. This gate does not attest that an operator supplied the
-- newest journal; freshness/hash must come from outside the restored backup.
CREATE TABLE dream.recovery_gate(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),ready boolean NOT NULL,expected_hash text NOT NULL);
INSERT INTO dream.recovery_gate VALUES(true,true,'');
CREATE TABLE dream.restored_revocations(actor text NOT NULL,namespace text NOT NULL,event_id text NOT NULL,PRIMARY KEY(actor,namespace,event_id));
REVOKE ALL ON dream.recovery_gate,dream.restored_revocations FROM PUBLIC,dream_actor,dream_private_reader,dream_research_reader;
GRANT SELECT,UPDATE ON dream.recovery_gate TO dream_writer;
GRANT SELECT,INSERT ON dream.restored_revocations TO dream_writer;
GRANT SELECT(ready,singleton) ON dream.recovery_gate TO dream_actor,dream_private_reader,dream_research_reader;
