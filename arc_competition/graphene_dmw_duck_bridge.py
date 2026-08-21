#!/usr/bin/env python3
"""GrapheneDMW sidecar for the ARC-AGI-3 Duck harness.

The bridge deliberately does not replace Duck's proven action loop.  It owns
only durable evidence, negative-action memory and optional dialectical
escalation.  This makes baseline, evidence-memory and full-DMW conditions
cleanly ablatable.
"""
from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Iterable


def _stable_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), default=str)


def digest(value: Any) -> str:
    return hashlib.sha256(_stable_json(value).encode("utf-8")).hexdigest()


def normalize_grid(grid: Any) -> tuple[tuple[int, ...], ...]:
    rows: list[tuple[int, ...]] = []
    for row in grid or []:
        if not isinstance(row, (list, tuple)):
            continue
        rows.append(tuple(int(cell) for cell in row))
    return tuple(rows)


def changed_cells(before: Iterable[Iterable[int]], after: Iterable[Iterable[int]]) -> int:
    a, b = list(map(tuple, before)), list(map(tuple, after))
    if len(a) != len(b):
        return -1
    total = 0
    for ra, rb in zip(a, b):
        if len(ra) != len(rb):
            return -1
        total += sum(x != y for x, y in zip(ra, rb))
    return total


@dataclass
class Transition:
    turn: int
    action: str
    before_digest: str
    after_digest: str
    changed_cells: int
    level_before: int | None = None
    level_after: int | None = None
    prediction_miss: bool = False

    @property
    def no_op(self) -> bool:
        return self.changed_cells == 0

    @property
    def level_transition(self) -> bool:
        return (
            self.level_before is not None
            and self.level_after is not None
            and self.level_after > self.level_before
        )


@dataclass
class DMWState:
    scientist_note: dict[str, str] = field(default_factory=dict)
    transitions: list[dict[str, Any]] = field(default_factory=list)
    negative_signatures: dict[str, int] = field(default_factory=dict)
    dialectic_events: list[dict[str, Any]] = field(default_factory=list)


class GrapheneDMWDuckBridge:
    MODES = {"off", "evidence", "dialectic"}

    def __init__(self, state_path: str | Path, *, mode: str = "evidence") -> None:
        if mode not in self.MODES:
            raise ValueError(f"unsupported GrapheneDMW mode {mode!r}")
        self.mode = mode
        self.state_path = Path(state_path)
        self.state = self._load()

    def _load(self) -> DMWState:
        if not self.state_path.exists():
            return DMWState()
        raw = json.loads(self.state_path.read_text(encoding="utf-8"))
        return DMWState(
            scientist_note=dict(raw.get("scientist_note") or {}),
            transitions=list(raw.get("transitions") or []),
            negative_signatures={str(k): int(v) for k, v in (raw.get("negative_signatures") or {}).items()},
            dialectic_events=list(raw.get("dialectic_events") or []),
        )

    def _save(self) -> None:
        self.state_path.parent.mkdir(parents=True, exist_ok=True)
        payload = {
            "scientist_note": self.state.scientist_note,
            "transitions": self.state.transitions[-256:],
            "negative_signatures": self.state.negative_signatures,
            "dialectic_events": self.state.dialectic_events[-128:],
        }
        tmp = self.state_path.with_suffix(self.state_path.suffix + ".tmp")
        tmp.write_text(json.dumps(payload, indent=2, sort_keys=True), encoding="utf-8")
        tmp.replace(self.state_path)

    def ingest_scientist_note(self, note: dict[str, Any]) -> None:
        if self.mode == "off":
            return
        allowed = {
            "world_model", "goal_model", "action_model", "recent_findings",
            "open_questions", "current_plan", "cross_level_notes",
        }
        for key in allowed:
            value = " ".join(str(note.get(key) or "").split())
            if value:
                self.state.scientist_note[key] = value
        self._save()

    @staticmethod
    def action_signature(before_digest: str, action: str) -> str:
        return digest({"state": before_digest, "action": action})

    def record_transition(
        self,
        *,
        turn: int,
        action: str,
        before_grid: Any,
        after_grid: Any,
        level_before: int | None = None,
        level_after: int | None = None,
        prediction_miss: bool = False,
    ) -> Transition:
        before = normalize_grid(before_grid)
        after = normalize_grid(after_grid)
        event = Transition(
            turn=int(turn),
            action=str(action),
            before_digest=digest(before),
            after_digest=digest(after),
            changed_cells=changed_cells(before, after),
            level_before=level_before,
            level_after=level_after,
            prediction_miss=bool(prediction_miss),
        )
        if self.mode != "off":
            self.state.transitions.append(event.__dict__)
            signature = self.action_signature(event.before_digest, event.action)
            if event.no_op:
                self.state.negative_signatures[signature] = self.state.negative_signatures.get(signature, 0) + 1
            elif signature in self.state.negative_signatures:
                # A previously negative signature changed state; evidence supersedes
                # the stale dead-action assumption rather than making it permanent.
                del self.state.negative_signatures[signature]
            self._save()
        return event

    def negative_count(self, before_grid: Any, action: str) -> int:
        before_digest = digest(normalize_grid(before_grid))
        return self.state.negative_signatures.get(self.action_signature(before_digest, action), 0)

    def stagnation_reasons(self) -> list[str]:
        if self.mode == "off":
            return []
        recent = [Transition(**row) for row in self.state.transitions[-4:]]
        reasons: list[str] = []
        if len(recent) >= 2 and all(item.no_op for item in recent[-2:]):
            reasons.append("two-consecutive-no-op-transitions")
        if any(item.prediction_miss for item in recent[-2:]):
            reasons.append("recent-forward-prediction-miss")
        if recent:
            last_sig = self.action_signature(recent[-1].before_digest, recent[-1].action)
            if self.state.negative_signatures.get(last_sig, 0) >= 2:
                reasons.append("repeated-dead-state-action-signature")
        return reasons

    def should_escalate_dialectic(self) -> bool:
        return self.mode == "dialectic" and bool(self.stagnation_reasons())

    def record_dialectic_receipt(self, receipt: dict[str, Any]) -> None:
        if self.mode != "dialectic":
            return
        safe = {
            "epistemic_status": receipt.get("epistemic_status"),
            "primary_hypothesis_id": receipt.get("primary_hypothesis_id"),
            "reopened_hypothesis_ids": list(receipt.get("reopened_hypothesis_ids") or []),
            "challenged_claims": list(receipt.get("challenged_claims") or []),
            "residual_uncertainty": list(receipt.get("residual_uncertainty") or []),
            "reasoning_receipt": dict(receipt.get("reasoning_receipt") or {}),
        }
        self.state.dialectic_events.append(safe)
        self._save()

    def prompt_context(self, *, max_negative_items: int = 5) -> str:
        if self.mode == "off":
            return ""
        lines = ["Graphene evidence memory (observed evidence outranks carried assumptions):"]
        note = self.state.scientist_note
        for key, label in [
            ("world_model", "Working model"),
            ("goal_model", "Working goal"),
            ("action_model", "Action model"),
            ("open_questions", "Open questions"),
            ("cross_level_notes", "Cross-level notes"),
        ]:
            if note.get(key):
                lines.append(f"- {label}: {note[key]}")

        recent_negative: list[tuple[str, int]] = sorted(
            self.state.negative_signatures.items(), key=lambda item: (-item[1], item[0])
        )[:max_negative_items]
        if recent_negative:
            lines.append("- Negative causal evidence: do not repeat a state/action signature unless new evidence justifies retesting.")
            lines.extend(f"  - signature {sig[:12]}... produced no observable grid change {count} time(s)" for sig, count in recent_negative)

        reasons = self.stagnation_reasons()
        if self.mode == "dialectic" and reasons:
            lines.append("- DIALECTIC ESCALATION ACTIVE: current interpretation may be in a false-convergence attractor.")
            lines.append(f"  Trigger: {', '.join(reasons)}")
            lines.append("  Preserve at least one alternative explanation and choose the cheapest discriminating test before committing to a long plan.")
        if self.state.dialectic_events:
            last = self.state.dialectic_events[-1]
            reopened = last.get("reopened_hypothesis_ids") or []
            challenged = last.get("challenged_claims") or []
            if reopened:
                lines.append(f"- Native DMW reopened alternatives: {', '.join(map(str, reopened))}")
            if challenged:
                lines.append(f"- Native DMW challenged claims: {' | '.join(map(str, challenged[:3]))}")
        return "\n".join(lines)

    def build_native_proposal(
        self,
        *,
        turn: int,
        observation_statement: str,
        observation_ref: str,
        hypotheses: list[tuple[str, str]],
        goal_statement: str,
        action: str,
        predicted_observation: str,
        falsification_questions: list[str],
    ) -> dict[str, Any]:
        """Build canonical A1.5 schema without asking Duck to emit database JSON.

        `hypotheses` is [(statement, prediction), ...]. The caller must provide at
        least two genuinely different semantic alternatives before invoking native
        opposition. The bridge assigns IDs/provenance deterministically.
        """
        if len(hypotheses) < 2:
            raise ValueError("dialectic escalation requires at least two competing hypotheses")
        normalized = [(" ".join(s.split()), " ".join(p.split())) for s, p in hypotheses]
        if len({s.lower() for s, _ in normalized}) < 2:
            raise ValueError("competing hypotheses must be semantically distinct strings")
        oid = f"t{turn}-o1"
        hs = [
            {
                "id": f"t{turn}-h{i}",
                "statement": statement,
                "prediction": prediction,
                "basis": [{"observation_id": oid}],
                "status": "proposed",
            }
            for i, (statement, prediction) in enumerate(normalized, start=1)
        ]
        gid = f"t{turn}-g1"
        return {
            "turn": int(turn),
            "observations": [{
                "id": oid,
                "statement": " ".join(observation_statement.split()),
                "evidence_ref": str(observation_ref),
                "evidence_kind": "grid_transition",
            }],
            "hypotheses": hs,
            "candidate_goals": [{
                "id": gid,
                "statement": " ".join(goal_statement.split()),
                "basis": [{"hypothesis_id": hs[0]["id"], "observation_id": oid}],
                "status": "proposed",
            }],
            "opposition": {"falsification_questions": [" ".join(q.split()) for q in falsification_questions if q.strip()]},
            "experiment": {
                "tests_hypothesis_ids": [h["id"] for h in hs],
                "action": str(action),
                "action_params": {},
                "information_goal": "Distinguish the competing world-model hypotheses using observable environment evidence.",
                "predicted_observation": " ".join(predicted_observation.split()),
            },
            "residual_uncertainty": ["The hidden game mechanics and objective remain provisional until falsified or repeatedly supported."],
        }
