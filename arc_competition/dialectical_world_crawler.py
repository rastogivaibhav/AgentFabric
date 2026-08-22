#!/usr/bin/env python3
from __future__ import annotations
from dataclasses import dataclass, field
from hashlib import sha256
from itertools import product
from typing import Callable, Iterable, Any
import json, math

Frame = list[list[int]]
Action = Any


def frame_hash(frame: Frame) -> str:
    return sha256(json.dumps(frame,separators=(',',':')).encode()).hexdigest()[:16]


def connected_components(frame: Frame) -> list[dict[str,Any]]:
    h=len(frame); w=len(frame[0]) if h else 0; seen=set(); out=[]
    for y in range(h):
        for x in range(w):
            if (x,y) in seen: continue
            colour=frame[y][x]
            stack=[(x,y)]; seen.add((x,y)); pts=[]
            while stack:
                px,py=stack.pop(); pts.append((px,py))
                for nx,ny in ((px+1,py),(px-1,py),(px,py+1),(px,py-1)):
                    if 0<=nx<w and 0<=ny<h and (nx,ny) not in seen and frame[ny][nx]==colour:
                        seen.add((nx,ny)); stack.append((nx,ny))
            xs=[p[0] for p in pts]; ys=[p[1] for p in pts]
            out.append({'colour':colour,'area':len(pts),'bbox':[min(xs),min(ys),max(xs),max(ys)],'centroid':[sum(xs)/len(xs),sum(ys)/len(ys)]})
    return out


def frame_delta(before: Frame, after: Frame) -> dict[str,Any]:
    changed=[]
    for y,row in enumerate(before):
        for x,v in enumerate(row):
            if after[y][x]!=v: changed.append([x,y,v,after[y][x]])
    return {'changed_pixels':changed,'changed_count':len(changed),'before_hash':frame_hash(before),'after_hash':frame_hash(after)}

@dataclass
class RuleStats:
    n:int=0
    outcome_counts:dict[str,int]=field(default_factory=dict)
    def observe(self, outcome:str):
        self.n+=1; self.outcome_counts[outcome]=self.outcome_counts.get(outcome,0)+1
    @property
    def confidence(self)->float:
        if not self.n: return 0.0
        return max(self.outcome_counts.values())/self.n
    @property
    def entropy(self)->float:
        if not self.n: return 1.0
        e=0.0
        for c in self.outcome_counts.values():
            p=c/self.n; e-=p*math.log2(p)
        return e

class DialecticalWorldCrawler:
    def __init__(self, actions:Iterable[Action], max_depth:int=3, saturation_confidence:float=.95):
        self.actions=list(actions); self.max_depth=max_depth; self.saturation_confidence=saturation_confidence
        self.rules:dict[tuple[str,...],RuleStats]={}; self.transitions=[]; self.graphene_records=[]

    def record(self, before:Frame, sequence:list[Action], after:Frame, terminal:bool=False):
        d=frame_delta(before,after); outcome=json.dumps({'delta':d['changed_pixels'],'terminal':terminal},separators=(',',':'))
        key=tuple(map(str,sequence)); self.rules.setdefault(key,RuleStats()).observe(outcome)
        rec={'kind':'Transition','state_before':d['before_hash'],'actions':list(sequence),'state_after':d['after_hash'],'delta':d,'terminal':terminal,'entities_before':connected_components(before),'entities_after':connected_components(after)}
        self.transitions.append(rec)
        hid='rule:'+sha256(('|'.join(key)).encode()).hexdigest()[:12]
        self.graphene_records.extend([
            {'kind':'Observation','id':'obs:'+sha256(json.dumps(rec,sort_keys=True).encode()).hexdigest()[:12],'payload':rec},
            {'kind':'Hypothesis','id':hid,'statement':f"sequence {key} predicts observed transition class",'support':self.rules[key].confidence,'samples':self.rules[key].n,'status':'supported' if self.rules[key].confidence>=self.saturation_confidence else 'contested'}
        ])
        return rec

    def information_score(self, sequence:list[Action])->float:
        s=self.rules.get(tuple(map(str,sequence)))
        if s is None: return 1.0 + 0.15*len(sequence)
        novelty=1/(1+s.n)
        uncertainty=(1-s.confidence)+min(1.0,s.entropy)
        return novelty+uncertainty-0.05*len(sequence)

    def candidate_sequences(self):
        for depth in range(1,self.max_depth+1):
            for seq in product(self.actions,repeat=depth):
                s=self.rules.get(tuple(map(str,seq)))
                if s and s.n>=2 and s.confidence>=self.saturation_confidence: continue
                yield list(seq)

    def next_experiment(self)->list[Action]|None:
        cands=list(self.candidate_sequences())
        if not cands: return None
        return max(cands,key=self.information_score)

    def crawl(self, reset:Callable[[],Frame], step:Callable[[Action],tuple[Frame,bool]], budget:int=100):
        for _ in range(budget):
            seq=self.next_experiment()
            if seq is None: break
            before=reset(); after=before; terminal=False
            for a in seq:
                after,terminal=step(a)
                if terminal: break
            self.record(before,seq,after,terminal)
        return {'transitions':len(self.transitions),'rules':len(self.rules),'remaining':self.next_experiment() is not None,'graphene_records':len(self.graphene_records)}
