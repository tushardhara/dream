-- Explicit login/actor/namespace scopes for privileged restricted readers.
-- The migration authority provisions these mappings. Runtime writers and readers
-- cannot grant themselves access. session_user is the authenticated DB login,
-- never a user-controlled role header or custom setting.
CREATE TABLE dream.reader_scopes(
 login name NOT NULL, actor text NOT NULL, namespace text NOT NULL,
 payload_class text NOT NULL CHECK(payload_class IN ('private','research')),
 PRIMARY KEY(login,actor,namespace,payload_class)
);
REVOKE ALL ON dream.reader_scopes FROM PUBLIC,dream_actor,dream_writer,dream_private_reader,dream_research_reader;
GRANT SELECT ON dream.reader_scopes TO dream_private_reader,dream_research_reader;
ALTER TABLE dream.reader_scopes ENABLE ROW LEVEL SECURITY;
ALTER TABLE dream.reader_scopes FORCE ROW LEVEL SECURITY;
CREATE POLICY reader_scope_self ON dream.reader_scopes FOR SELECT TO dream_private_reader,dream_research_reader USING(login=session_user);

ALTER TABLE dream.observable_payloads FORCE ROW LEVEL SECURITY;
ALTER TABLE dream.private_payloads ENABLE ROW LEVEL SECURITY;
ALTER TABLE dream.private_payloads FORCE ROW LEVEL SECURITY;
CREATE POLICY private_writer ON dream.private_payloads TO dream_writer USING(true) WITH CHECK(true);
CREATE POLICY private_scoped_reader ON dream.private_payloads FOR SELECT TO dream_private_reader USING(
 EXISTS(SELECT 1 FROM dream.reader_scopes s WHERE s.login=session_user AND s.actor=private_payloads.actor AND s.namespace=private_payloads.namespace AND s.payload_class='private')
);
ALTER TABLE dream.research_payloads ENABLE ROW LEVEL SECURITY;
ALTER TABLE dream.research_payloads FORCE ROW LEVEL SECURITY;
CREATE POLICY research_writer ON dream.research_payloads TO dream_writer USING(true) WITH CHECK(true);
CREATE POLICY research_scoped_reader ON dream.research_payloads FOR SELECT TO dream_research_reader USING(
 EXISTS(SELECT 1 FROM dream.reader_scopes s WHERE s.login=session_user AND s.actor=research_payloads.actor AND s.namespace=research_payloads.namespace AND s.payload_class='research')
);
-- No reader receives runtime checkpoint/operation privileges. Research views go
-- through explicit application capabilities rather than direct GodState tables.
ALTER TABLE dream.runtime_payloads ENABLE ROW LEVEL SECURITY;
ALTER TABLE dream.runtime_payloads FORCE ROW LEVEL SECURITY;
CREATE POLICY runtime_writer ON dream.runtime_payloads TO dream_writer USING(true) WITH CHECK(true);
INSERT INTO dream.schema_versions VALUES(3);
