"""Durable, fail-closed AgentKraft mission coordination for Perplexity SPACE.

This is a single-host SQLite reference broker, not a distributed lease manager.
It stores correlation metadata and hashes, never LLM prompts, OAuth tokens or
raw tool outputs. Real tenant authorization must happen before entering this
trusted control-plane boundary. See README for explicit GA limitations.
"""
from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from hashlib import sha256
import json
import os
from pathlib import Path
import re
import sqlite3
from typing import Callable

from space_client import SandboxRef, SpaceComputerProvider, CommandResult

_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$")


def _identity(value: str, name: str) -> str:
    if not isinstance(value, str) or not _ID.fullmatch(value):
        raise ValueError(f"{name} must be a stable, nonsecret identifier")
    return value


def _canonical(value: object) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def _digest(value: object) -> str:
    return sha256(_canonical(value).encode("utf-8")).hexdigest()


class MissionStateError(RuntimeError):
    """A run is ambiguous, unauthorised, or in the wrong lifecycle state."""


@dataclass(frozen=True)
class Permit:
    """Result from a trusted out-of-VM authorization service (not yet AGS-wired)."""
    allow: bool
    decision_ref: str


@dataclass(frozen=True)
class Mission:
    tenant_id: str
    run_id: str
    state: str
    sandbox_id: str | None
    operation_id: str | None


class MissionJournal:
    """SQLite reference with atomic transitions and tenant-scoped evidence chain.

    The chain detects offline record changes; it is NOT tamper proof against a
    database administrator who can rewrite all records or the anchor. Enterprise
    durability needs an external append-only anchor and a multi-node datastore.
    """

    def __init__(self, path: str | Path):
        path = Path(path)
        if str(path) == ":memory:":
            raise ValueError("In-memory journals cannot protect VM lifecycle")
        path.parent.mkdir(parents=True, exist_ok=True)
        if path.exists() and path.is_symlink():
            raise ValueError("Refusing symlink journal path")
        if not path.exists():
            fd = os.open(str(path), os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
            os.close(fd)
        os.chmod(path, 0o600)
        self._db = sqlite3.connect(str(path), timeout=20, isolation_level="IMMEDIATE")
        self._db.execute("PRAGMA journal_mode=WAL")
        self._db.execute("PRAGMA synchronous=FULL")
        self._db.execute("PRAGMA foreign_keys=ON")
        self._db.execute("""
            CREATE TABLE IF NOT EXISTS missions (
                tenant_id TEXT NOT NULL, run_id TEXT NOT NULL, intent_sha256 TEXT NOT NULL,
                state TEXT NOT NULL, sandbox_id TEXT, operation_id TEXT,
                pending_kind TEXT, pending_key TEXT, pending_fingerprint TEXT,
                PRIMARY KEY (tenant_id, run_id)
            )""")
        self._db.execute("""
            CREATE TABLE IF NOT EXISTS evidence (
                tenant_id TEXT NOT NULL, run_id TEXT NOT NULL, seq INTEGER NOT NULL,
                event TEXT NOT NULL, metadata TEXT NOT NULL,
                previous_hash TEXT NOT NULL, event_hash TEXT NOT NULL,
                PRIMARY KEY (tenant_id, run_id, seq),
                FOREIGN KEY (tenant_id, run_id) REFERENCES missions(tenant_id, run_id)
            )""")
        self._db.commit()

    def close(self):
        self._db.close()

    def _event(self, tenant: str, run: str, event: str, metadata: dict):
        if not isinstance(metadata, dict):
            raise ValueError("evidence metadata must be an object")
        # Never persist token/body/command text in the evidence log.
        allowed = {"intent_sha256", "fingerprint", "kind", "decision_ref",
                   "sandbox_id", "operation_id", "process_id", "exit_code", "reason"}
        if set(metadata) - allowed:
            raise ValueError("evidence contains disallowed fields")
        metadata = {**metadata, "at_utc": datetime.now(timezone.utc).isoformat()}
        row = self._db.execute(
            "SELECT seq,event_hash FROM evidence WHERE tenant_id=? AND run_id=? ORDER BY seq DESC LIMIT 1",
            (tenant, run),
        ).fetchone()
        seq, previous = (row[0] + 1, row[1]) if row else (1, "GENESIS")
        body = _canonical({"tenant_id": tenant, "run_id": run, "seq": seq,
                           "event": event, "metadata": metadata, "previous_hash": previous})
        digest = sha256(body.encode()).hexdigest()
        self._db.execute(
            "INSERT INTO evidence VALUES (?,?,?,?,?,?,?)",
            (tenant, run, seq, event, _canonical(metadata), previous, digest),
        )

    def begin(self, tenant_id: str, run_id: str, intent: str) -> Mission:
        tenant, run = _identity(tenant_id, "tenant_id"), _identity(run_id, "run_id")
        if not isinstance(intent, str) or not intent.strip() or len(intent) > 20000:
            raise ValueError("intent must be a non-empty description <= 20000 characters")
        intent_hash = sha256(intent.encode()).hexdigest()
        with self._db:
            self._db.execute(
                "INSERT INTO missions (tenant_id,run_id,intent_sha256,state) VALUES (?,?,?,?)",
                (tenant, run, intent_hash, "new"),
            )
            self._event(tenant, run, "mission.created", {"intent_sha256": intent_hash})
        return self.get(tenant, run)

    def get(self, tenant_id: str, run_id: str) -> Mission:
        tenant, run = _identity(tenant_id, "tenant_id"), _identity(run_id, "run_id")
        row = self._db.execute(
            "SELECT state,sandbox_id,operation_id FROM missions WHERE tenant_id=? AND run_id=?",
            (tenant, run),
        ).fetchone()
        if not row:
            raise MissionStateError("Unknown mission in tenant scope")
        return Mission(tenant, run, *row)

    def reserve(self, tenant: str, run: str, *, expected: str, kind: str, key: str, payload: dict):
        _identity(key, "request_key")
        fingerprint = _digest(payload)
        with self._db:
            state = self.get(tenant, run).state
            if state != expected:
                raise MissionStateError(f"Mission state {state} disallows {kind}; inspect before retry")
            self._db.execute(
                "UPDATE missions SET state=?,pending_kind=?,pending_key=?,pending_fingerprint=? "
                "WHERE tenant_id=? AND run_id=?",
                ("pending_" + kind, kind, key, fingerprint, tenant, run),
            )
            self._event(tenant, run, kind + ".reserved", {"kind": kind, "fingerprint": fingerprint})
        return fingerprint

    def finish(self, tenant: str, run: str, *, kind: str, new_state: str,
               metadata: dict, sandbox_id: str | None = None, operation_id: str | None = None):
        with self._db:
            state = self.get(tenant, run).state
            if state != "pending_" + kind:
                raise MissionStateError("Unexpected pending operation; refusing completion")
            self._db.execute(
                "UPDATE missions SET state=?,sandbox_id=COALESCE(?,sandbox_id),"
                "operation_id=COALESCE(?,operation_id),pending_kind=NULL,"
                "pending_key=NULL,pending_fingerprint=NULL WHERE tenant_id=? AND run_id=?",
                (new_state, sandbox_id, operation_id, tenant, run),
            )
            self._event(tenant, run, kind + ".completed", metadata)

    def uncertain(self, tenant: str, run: str, *, kind: str):
        with self._db:
            if self.get(tenant, run).state != "pending_" + kind:
                return
            self._db.execute(
                "UPDATE missions SET state='needs_reconciliation' WHERE tenant_id=? AND run_id=?",
                (tenant, run),
            )
            self._event(tenant, run, kind + ".uncertain", {"kind": kind})

    def reject(self, tenant: str, run: str, *, kind: str, decision_ref: str):
        if kind not in ("command", "create"):
            raise ValueError("Only create and command denials are journalled")
        decision_ref = _identity(decision_ref, "decision_ref")
        with self._db:
            self._event(tenant, run, kind + ".denied", {"decision_ref": decision_ref})

    def events(self, tenant_id: str, run_id: str) -> list[dict]:
        tenant, run = _identity(tenant_id, "tenant_id"), _identity(run_id, "run_id")
        self.get(tenant, run)
        rows = self._db.execute(
            "SELECT seq,event,metadata,previous_hash,event_hash FROM evidence "
            "WHERE tenant_id=? AND run_id=? ORDER BY seq",
            (tenant, run),
        ).fetchall()
        return [{"seq": n, "event": event, "metadata": json.loads(meta),
                 "previous_hash": prev, "event_hash": digest}
                for n, event, meta, prev, digest in rows]

    def verify_chain(self, tenant: str, run: str) -> bool:
        previous = "GENESIS"
        for row in self.events(tenant, run):
            body = _canonical({"tenant_id": tenant, "run_id": run, "seq": row["seq"],
                               "event": row["event"], "metadata": row["metadata"],
                               "previous_hash": row["previous_hash"]})
            if row["previous_hash"] != previous or sha256(body.encode()).hexdigest() != row["event_hash"]:
                return False
            previous = row["event_hash"]
        return True


class MissionBroker:
    """Trusted external control-plane; NEVER execute inside the agent's VM."""

    def __init__(self, journal: MissionJournal, provider: SpaceComputerProvider,
                 authorize_command: Callable[[str, str, str], Permit] | None = None,
                 authorize_launch: Callable[[str, str, str], Permit] | None = None):
        self.journal = journal
        self.provider = provider
        self.authorize_command = authorize_command
        self.authorize_launch = authorize_launch

    def launch(self, tenant: str, run: str, *, template: str, request_key: str,
               network_policy: dict | None = None) -> Mission:
        # Check tenant scope before asking an authorizer to approve a VM.
        if self.journal.get(tenant, run).state != "new":
            raise MissionStateError("Mission is not new; inspect before launching another VM")
        payload = {"template": template, "runtime": "fc",
                   "network_policy": network_policy or
                   {"version": "v1", "egress": {"default_action": "deny", "allow": [], "deny": []}}}
        # VM allocation consumes resources; an explicit trusted launch decision is
        # mandatory even for a sandbox with default-deny external networking.
        if self.authorize_launch is None:
            self.journal.reject(tenant, run, kind="create", decision_ref="NO_LAUNCH_AUTHORISER")
            raise MissionStateError("No trusted launch authorization adapter; VM creation denied")
        spec_digest = _digest(payload)
        try:
            permit = self.authorize_launch(tenant, run, spec_digest)
        except Exception as exc:
            self.journal.reject(tenant, run, kind="create", decision_ref="AUTHORIZATION_ERROR")
            raise MissionStateError("Launch authorization unavailable; VM creation denied") from exc
        if not isinstance(permit, Permit) or type(permit.allow) is not bool:
            self.journal.reject(tenant, run, kind="create", decision_ref="INVALID_DECISION")
            raise MissionStateError("Invalid launch decision; VM creation denied")
        if not permit.allow or not permit.decision_ref:
            self.journal.reject(tenant, run, kind="create", decision_ref="DENIED")
            raise MissionStateError("Launch explicitly denied by authorization boundary")
        decision_ref = _identity(permit.decision_ref, "decision_ref")
        # Persist the original request key and fingerprint before network I/O.
        self.journal.reserve(tenant, run, expected="new", kind="create", key=request_key,
                             payload={**payload, "decision_ref": decision_ref})
        try:
            ref = self.provider.create(template=template, request_key=request_key,
                                       network_policy=payload["network_policy"])
        except Exception:
            self.journal.uncertain(tenant, run, kind="create")
            raise
        self.journal.finish(tenant, run, kind="create", new_state="created",
                            sandbox_id=ref.sandbox_id, operation_id=ref.create_operation_id,
                            metadata={"sandbox_id": ref.sandbox_id, "operation_id": ref.create_operation_id,
                                      "decision_ref": decision_ref})
        try:
            self.provider.wait_ready(ref)
        except Exception:
            # Created sandbox is retained and visible; cleanup must be explicitly
            # reconciled. Do not provision a replacement VM behind the user's back.
            raise
        with self.journal._db:
            if self.journal.get(tenant, run).state != "created":
                raise MissionStateError("Mission state changed during readiness check")
            self.journal._db.execute(
                "UPDATE missions SET state='ready' WHERE tenant_id=? AND run_id=?", (tenant, run))
            self.journal._event(tenant, run, "sandbox.ready", {"sandbox_id": ref.sandbox_id})
        return self.journal.get(tenant, run)

    def execute(self, tenant: str, run: str, *, argv: list[str], request_key: str) -> CommandResult:
        mission = self.journal.get(tenant, run)
        if mission.state != "ready" or not mission.sandbox_id:
            raise MissionStateError("Sandbox is not ready; reconcile the mission before execution")
        # No authorization adapter is more secure than a fake "approved" response.
        if not self.authorize_command:
            self.journal.reject(tenant, run, kind="command", decision_ref="NO_COMMAND_AUTHORISER")
            raise MissionStateError("No trusted authorization adapter; command denied")
        if not isinstance(argv, list) or not argv or not all(isinstance(x, str) and x for x in argv):
            raise ValueError("Command argv must be a non-empty list")
        fingerprint = _digest(argv)
        try:
            permit = self.authorize_command(tenant, run, fingerprint)
        except Exception as exc:
            self.journal.reject(tenant, run, kind="command", decision_ref="AUTHORIZATION_ERROR")
            raise MissionStateError("Authorization unavailable; command denied") from exc
        if not isinstance(permit, Permit) or type(permit.allow) is not bool:
            self.journal.reject(tenant, run, kind="command", decision_ref="INVALID_DECISION")
            raise MissionStateError("Authorization failed closed")
        if not permit.allow or not permit.decision_ref:
            self.journal.reject(tenant, run, kind="command", decision_ref="DENIED")
            raise MissionStateError("Execution explicitly denied by authorization boundary")
        decision_ref = _identity(permit.decision_ref, "decision_ref")
        # The permit is scoped to this exact argv hash. No arbitrary command
        # substitutions after authorization.
        self.journal.reserve(tenant, run, expected="ready", kind="command",
                             key=request_key, payload={"argv_sha256": fingerprint, "decision_ref": decision_ref})
        try:
            result = self.provider.command(mission.sandbox_id, argv=argv, request_key=request_key)
        except Exception:
            self.journal.uncertain(tenant, run, kind="command")
            raise
        self.journal.finish(tenant, run, kind="command", new_state="ready",
                            metadata={"decision_ref": decision_ref, "process_id": result.process_id,
                                      "exit_code": result.exit_code})
        return result

    def terminate(self, tenant: str, run: str, *, request_key: str):
        mission = self.journal.get(tenant, run)
        if mission.state not in ("ready", "created") or not mission.sandbox_id:
            raise MissionStateError("Mission requires inspection before another delete attempt")
        self.journal.reserve(tenant, run, expected=mission.state, kind="delete", key=request_key,
                             payload={"sandbox_id": mission.sandbox_id})
        try:
            self.provider.delete(mission.sandbox_id, request_key=request_key)
        except Exception:
            self.journal.uncertain(tenant, run, kind="delete")
            raise
        self.journal.finish(tenant, run, kind="delete", new_state="deleted",
                            metadata={"sandbox_id": mission.sandbox_id})
