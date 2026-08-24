#!/usr/bin/env python3
"""GrapheneDMW sidecar for the ARC-AGI-3 Duck harness.

The bridge owns durable evidence, exact negative-action memory, a visual causal
world-crawler, and optional dialectical escalation. Raw ARC grids are converted
into structured transition evidence here rather than relying on the language
model to narrate what changed.
"""
from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Iterable

try:
    from inference.agent.dialectical_world_crawler import connected_components, frame_delta
except ModuleNotFoundError:
    try:
        from dialectical_world_crawler import connected_components, frame_delta
    except ModuleNotFoundError:
        connected_components = None
        frame_delta = None


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
        return self.level_before is not None and self.level_after is not None and self.level_after > self.level_before


@dataclass
class DMWState:
    scientist_note: dict[str, str] = field(default_factory=dict)
    transitions: list[dict[str, Any]] = field(default_factory=list)
    negative_signatures: dict[str, int] = field(default_factory=dict)
    dialectic_events: list[dict[str, Any]] = field(default_factory=list)
    crawler_records: list[dict[str, Any]] = field(default_factory=list)
    crawler_action_history: list[dict[str, Any]] = field(default_factory=list)
    crawler_sequence_stats: dict[str, dict[str, Any]] = field(default_factory=dict)


class GrapheneDMWDuckBridge:
    MODES = {"off", "evidence", "dialectic"}
    MAX_CRAWL_DEPTH = 6

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
            crawler_records=list(raw.get("crawler_records") or []),
            crawler_action_history=list(raw.get("crawler_action_history") or []),
            crawler_sequence_stats=dict(raw.get("crawler_sequence_stats") or {}),
        )

    def _save(self) -> None:
        self.state_path.parent.mkdir(parents=True, exist_ok=True)
        payload = {
            "scientist_note": self.state.scientist_note,
            "transitions": self.state.transitions[-256:],
            "negative_signatures": self.state.negative_signatures,
            "dialectic_events": self.state.dialectic_events[-128:],
            "crawler_records": self.state.crawler_records[-512:],
            "crawler_action_history": self.state.crawler_action_history[-self.MAX_CRAWL_DEPTH:],
            "crawler_sequence_stats": self.state.crawler_sequence_stats,
        }
        tmp = self.state_path.with_suffix(self.state_path.suffix + ".tmp")
        tmp.write_text(json.dumps(payload, indent=2, sort_keys=True), encoding="utf-8")
        tmp.replace(self.state_path)

    def ingest_scientist_note(self, note: dict[str, Any]) -> None:
        if self.mode == "off":
            return
        allowed = {"world_model", "goal_model", "action_model", "recent_findings", "open_questions", "current_plan", "cross_level_notes"}
        for key in allowed:
            value = " ".join(str(note.get(key) or "").split())
            if value:
                self.state.scientist_note[key] = value
        self._save()

    @staticmethod
    def canonical_action(actions: Any) -> str:
        return _stable_json(actions)

    @staticmethod
    def action_signature(before_digest: str, action: str) -> str:
        return digest({"state": before_digest, "action": action})

    def _record_crawler_evidence(self, *, turn: int, action: str, before: tuple[tuple[int, ...], ...], after: tuple[tuple[int, ...], ...], level_before: int | None, level_after: int | None) -> None:
        if self.mode == "off":
            return
        b=[list(r) for r in before]; a=[list(r) for r in after]
        if frame_delta is not None:
            delta=frame_delta(b,a)
        else:
            delta={"changed_pixels":[],"changed_count":changed_cells(before,after),"before_hash":digest(before)[:16],"after_hash":digest(after)[:16]}
        eb=connected_components(b) if connected_components is not None else []
        ea=connected_components(a) if connected_components is not None else []
        terminal=bool(level_before is not None and level_after is not None and level_after>level_before)
        event={
            "kind":"CrawlerObservation","turn":int(turn),"action":str(action),
            "before_hash":delta.get("before_hash"),"after_hash":delta.get("after_hash"),
            "changed_count":int(delta.get("changed_count") or 0),
            "changed_pixels":list(delta.get("changed_pixels") or [])[:256],
            "entities_before":eb[:64],"entities_after":ea[:64],
            "component_count_before":len(eb),"component_count_after":len(ea),
            "level_transition":terminal,
        }
        self.state.crawler_records.append(event)
        self.state.crawler_action_history.append({"turn":int(turn),"action":str(action),"changed_count":event["changed_count"],"terminal":terminal})
        self.state.crawler_action_history=self.state.crawler_action_history[-self.MAX_CRAWL_DEPTH:]
        hist=self.state.crawler_action_history
        # Every suffix is a candidate delayed-effect rule. Silent prefixes remain unresolved rather than discarded.
        for depth in range(1,min(self.MAX_CRAWL_DEPTH,len(hist))+1):
            suffix=hist[-depth:]
            key=" -> ".join(x["action"] for x in suffix)
            stat=self.state.crawler_sequence_stats.setdefault(key,{"depth":depth,"samples":0,"changed":0,"terminal":0,"silent":0,"last_turn":turn})
            stat["samples"]+=1; stat["last_turn"]=turn
            if event["changed_count"]>0: stat["changed"]+=1
            else: stat["silent"]+=1
            if terminal: stat["terminal"]+=1
        # Graphene-ready hypothesis record, deliberately provisional until repeated/falsified.
        seq=" -> ".join(x["action"] for x in hist)
        self.state.crawler_records.append({
            "kind":"Hypothesis","id":"crawler-h-"+digest({"seq":seq,"turn":turn})[:12],
            "statement":f"Recent sequence [{seq}] may explain the observed transition; preserve silent prefixes as possible delayed preconditions.",
            "status":"supported" if event["changed_count"]>0 or terminal else "contested",
            "support":1.0 if terminal else (0.65 if event["changed_count"]>0 else 0.2),
            "depth":len(hist),"turn":turn,
        })

    def curiosity_targets(self, limit: int = 5) -> list[str]:
        stats=[]
        for seq,s in self.state.crawler_sequence_stats.items():
            n=max(1,int(s.get("samples") or 0)); changed=int(s.get("changed") or 0); silent=int(s.get("silent") or 0); terminal=int(s.get("terminal") or 0)
            # Voracious curiosity: low sample count + unresolved silence + depth get positive weight; terminal rules need less retesting.
            score=(1.0/n)+(0.45 if silent and not changed else 0)+(0.08*int(s.get("depth") or 1))-(1.0 if terminal else 0)
            stats.append((score,seq,s))
        stats.sort(key=lambda x:(-x[0],x[1]))
        out=[]
        for score,seq,s in stats[:limit]:
            out.append(f"{seq} [depth={s.get('depth')}, samples={s.get('samples')}, changed={s.get('changed')}, silent={s.get('silent')}, terminal={s.get('terminal')}, curiosity={score:.2f}]")
        return out

    def record_transition(self, *, turn: int, action: str, before_grid: Any, after_grid: Any, level_before: int | None = None, level_after: int | None = None, prediction_miss: bool = False) -> Transition:
        before=normalize_grid(before_grid); after=normalize_grid(after_grid)
        event=Transition(turn=int(turn),action=str(action),before_digest=digest(before),after_digest=digest(after),changed_cells=changed_cells(before,after),level_before=level_before,level_after=level_after,prediction_miss=bool(prediction_miss))
        if self.mode != "off":
            self.state.transitions.append(event.__dict__)
            signature=self.action_signature(event.before_digest,event.action)
            if event.no_op: self.state.negative_signatures[signature]=self.state.negative_signatures.get(signature,0)+1
            elif signature in self.state.negative_signatures: del self.state.negative_signatures[signature]
            self._record_crawler_evidence(turn=turn,action=action,before=before,after=after,level_before=level_before,level_after=level_after)
            self._save()
        return event

    def negative_count(self, before_grid: Any, action: str) -> int:
        if self.mode == "off": return 0
        return self.state.negative_signatures.get(self.action_signature(digest(normalize_grid(before_grid)),action),0)

    def dead_action_reason(self, before_grid: Any, action: str) -> str | None:
        count=self.negative_count(before_grid,action)
        if count<=0: return None
        return "Graphene negative evidence: this exact action in this exact grid state already produced no observable grid change %d time(s). Choose a different interaction or first change the state; retest only if new evidence justifies it."%count

    def stagnation_reasons(self) -> list[str]:
        if self.mode == "off": return []
        recent=[Transition(**row) for row in self.state.transitions[-4:]]; reasons=[]
        if len(recent)>=2 and all(item.no_op for item in recent[-2:]): reasons.append("two-consecutive-no-op-transitions")
        if any(item.prediction_miss for item in recent[-2:]): reasons.append("recent-forward-prediction-miss")
        if recent:
            last_sig=self.action_signature(recent[-1].before_digest,recent[-1].action)
            if self.state.negative_signatures.get(last_sig,0)>=2: reasons.append("repeated-dead-state-action-signature")
        return reasons

    def should_escalate_dialectic(self) -> bool:
        return self.mode == "dialectic" and bool(self.stagnation_reasons())

    def record_dialectic_receipt(self, receipt: dict[str, Any]) -> None:
        if self.mode != "dialectic": return
        safe={"epistemic_status":receipt.get("epistemic_status"),"primary_hypothesis_id":receipt.get("primary_hypothesis_id"),"reopened_hypothesis_ids":list(receipt.get("reopened_hypothesis_ids") or []),"challenged_claims":list(receipt.get("challenged_claims") or []),"residual_uncertainty":list(receipt.get("residual_uncertainty") or []),"reasoning_receipt":dict(receipt.get("reasoning_receipt") or {})}
        self.state.dialectic_events.append(safe); self._save()

    def prompt_context(self, *, max_negative_items: int = 5) -> str:
        if self.mode == "off": return ""
        lines=["Graphene evidence memory (observed evidence outranks carried assumptions):"]
        note=self.state.scientist_note
        for key,label in [("world_model","Working model"),("goal_model","Working goal"),("action_model","Action model"),("open_questions","Open questions"),("cross_level_notes","Cross-level notes")]:
            if note.get(key): lines.append(f"- {label}: {note[key]}")
        recent=[x for x in self.state.crawler_records if x.get("kind")=="CrawlerObservation"][-5:]
        if recent:
            lines.append("- WORLD CRAWLER: direct pixel-grounded causal observations:")
            for x in recent:
                lines.append(f"  - turn {x['turn']}: action {x['action']} changed {x['changed_count']} pixels; components {x['component_count_before']}→{x['component_count_after']}; level_transition={x['level_transition']}")
            targets=self.curiosity_targets(4)
            if targets:
                lines.append("- CURIOSITY FRONTIER: under-tested or silent sequences remain scientifically live; prefer cheap discriminating continuations:")
                lines.extend(f"  - {x}" for x in targets)
        recent_negative=sorted(self.state.negative_signatures.items(),key=lambda item:(-item[1],item[0]))[:max_negative_items]
        if recent_negative:
            lines.append("- Negative causal evidence: an exact state/action no-op is blocked from immediate repetition unless the state changes.")
        reasons=self.stagnation_reasons()
        if self.mode=="dialectic" and reasons:
            lines.append("- DIALECTIC ESCALATION ACTIVE: current interpretation may be in a false-convergence attractor.")
            lines.append(f"  Trigger: {', '.join(reasons)}")
            lines.append("  Preserve at least one alternative explanation and choose the cheapest discriminating test before committing to a long plan.")
        if self.state.dialectic_events:
            last=self.state.dialectic_events[-1]; reopened=last.get("reopened_hypothesis_ids") or []; challenged=last.get("challenged_claims") or []
            if reopened: lines.append(f"- Native DMW reopened alternatives: {', '.join(map(str,reopened))}")
            if challenged: lines.append(f"- Native DMW challenged claims: {' | '.join(map(str,challenged[:3]))}")
        return "\n".join(lines)

    def build_native_proposal(self, *, turn:int, observation_statement:str, observation_ref:str, hypotheses:list[tuple[str,str]], goal_statement:str, action:str, predicted_observation:str, falsification_questions:list[str], action_params:dict[str,Any]|None=None) -> dict[str,Any]:
        if len(hypotheses)<2: raise ValueError("dialectic escalation requires at least two competing hypotheses")
        normalized=[(" ".join(s.split())," ".join(p.split())) for s,p in hypotheses]
        if len({s.lower() for s,_ in normalized})<2: raise ValueError("competing hypotheses must be semantically distinct strings")
        questions=[" ".join(q.split()) for q in falsification_questions if q.strip()]
        if not questions: raise ValueError("dialectic escalation requires at least one falsification question")
        oid=f"t{turn}-o1"; hs=[{"id":f"t{turn}-h{i}","statement":statement,"prediction":prediction,"basis":[{"observation_id":oid}],"status":"proposed"} for i,(statement,prediction) in enumerate(normalized,start=1)]; gid=f"t{turn}-g1"
        return {"turn":int(turn),"observations":[{"id":oid,"statement":" ".join(observation_statement.split()),"evidence_ref":str(observation_ref),"evidence_kind":"transition"}],"hypotheses":hs,"candidate_goals":[{"id":gid,"statement":" ".join(goal_statement.split()),"basis":[{"hypothesis_id":hs[0]["id"],"observation_id":oid}],"status":"proposed"}],"opposition":{"falsification_questions":questions[:3]},"experiment":{"tests_hypothesis_ids":[h["id"] for h in hs],"action":str(action),"action_params":dict(action_params or {}),"information_goal":"Distinguish competing world-model hypotheses using observable environment evidence.","predicted_observation":" ".join(predicted_observation.split())},"residual_uncertainty":["Hidden mechanics and objective remain provisional until falsified or repeatedly supported."]}
