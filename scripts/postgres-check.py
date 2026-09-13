"""Mandatory bounded integration and backup/restore validation on disposable PG18.
No existing database/DSN is accepted. Never run a live provider or touch host data.
"""
import os
import pathlib
import subprocess
import time
import uuid

name = "dream-check-" + uuid.uuid4().hex[:12]

def command(*args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True, timeout=60, **kwargs).stdout.strip()

try:
    command("docker", "run", "--detach", "--rm", "--name", name,
            "--label", "dream.disposable=true", "--cpus", "2", "--memory", "512m",
            "--pids-limit", "128", "--tmpfs", "/var/lib/postgresql",
            "--publish", "127.0.0.1::5432", "--env", "POSTGRES_PASSWORD=disposable_local_only",
            "postgres:18.6")
    for _ in range(60):
        probe = subprocess.run(["docker", "exec", name, "pg_isready", "-U", "postgres"], capture_output=True)
        if probe.returncode == 0:
            break
        time.sleep(1)
    else:
        raise RuntimeError("disposable Postgres did not become ready")
    port = command("docker", "port", name, "5432/tcp").rsplit(":", 1)[1]
    env = dict(os.environ, DREAM_TEST_DSN=f"postgres://postgres:disposable_local_only@127.0.0.1:{port}/postgres?sslmode=disable")
    subprocess.run(["go", "test", "-race", "-count=1", "-timeout=120s", "-v", "./adapters/postgres"], env=env, check=True, timeout=150)
    dump = command("docker", "exec", name, "pg_dump", "-U", "postgres", "-d", "postgres", "--schema=dream")
    command("docker", "exec", name, "createdb", "-U", "postgres", "dream_restore")
    command("docker", "exec", "-i", name, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "dream_restore", input=dump)
    query = """SELECT (SELECT count(*) FROM dream.schema_versions WHERE version IN (1,2,3,4))=4 AND (SELECT max(version) FROM dream.schema_versions)=4
      AND (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE n.nspname='dream' AND c.relname IN ('observable_payloads','private_payloads','research_payloads','runtime_payloads','reader_scopes','model_payloads') AND c.relrowsecurity AND c.relforcerowsecurity)=6
      AND (SELECT count(*) FROM dream.tombstones)>0
      AND NOT EXISTS(SELECT 1 FROM (SELECT actor,namespace,event_id FROM dream.observable_payloads
      UNION ALL SELECT actor,namespace,event_id FROM dream.private_payloads
      UNION ALL SELECT actor,namespace,event_id FROM dream.research_payloads
      UNION ALL SELECT actor,namespace,event_id FROM dream.runtime_payloads
      UNION ALL SELECT actor,namespace,event_id FROM dream.model_payloads) p
      JOIN dream.tombstones t USING(actor,namespace,event_id))
      AND NOT EXISTS(SELECT 1 FROM dream.runtime_operations o JOIN dream.runtime_heads h USING(actor,namespace,run)
      JOIN dream.tombstones t ON (h.actor,h.namespace,h.event_id)=(t.actor,t.namespace,t.event_id));"""
    restored = command("docker", "exec", name, "psql", "-XAt", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "dream_restore", "-c", query)
    if restored != "t":
        raise RuntimeError("restored schema/tombstone/payload validation failed")
    print("PASS: forward migrations, real Postgres integration, and purged-state pg_dump/restore")
finally:
    subprocess.run(["docker", "rm", "--force", name], capture_output=True, timeout=30)
