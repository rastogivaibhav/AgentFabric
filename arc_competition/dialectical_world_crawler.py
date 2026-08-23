#!/usr/bin/env python3
from __future__ import annotations
from dataclasses import dataclass, field
from hashlib import sha256
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
            colour=frame[y][x]; stack=[(x,y)]; seen.add((x,y)); pts=[]
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
    n:int=0; outcome_counts:dict[str,int]=field(default_factory=dict); max_changed:int=0; ever_terminal:bool=False
    def observe(self,outcome:str,changed:int=0,terminal:bool=False):
        self.n+=1; self.outcome_counts[outcome]=self.outcome_counts.get(outcome,0)+1; self.max_changed=max(self.max_changed,changed); self.ever_terminal|=terminal
    @property
    def confidence(self): return max(self.outcome_counts.values())/self.n if self.n else 0.0
    @property
    def entropy(self):
        if not self.n: return 1.0
        return -sum((c/self.n)*math.log2(c/self.n) for c in self.outcome_counts.values())
    @property
    def interesting(self): return min(2.0,self.max_changed/2.0)+(1.0 if self.ever_terminal else 0.0)+(0.25 if self.max_changed==0 else 0.0)

class DialecticalWorldCrawler:
    def __init__(self,actions:Iterable[Action],max_depth:int=6,saturation_confidence:float=.95,beam_width:int=16,silent_quota:int=4):
        self.actions=list(actions); self.max_depth=max_depth; self.saturation_confidence=saturation_confidence; self.beam_width=beam_width; self.silent_quota=silent_quota
        self.rules={}; self.transitions=[]; self.graphene_records=[]; self._frontiers={1:[(str(a),) for a in self.actions]}; self._tested=set(); self._terminal_found=False
    def record(self,before,sequence,after,terminal=False):
        d=frame_delta(before,after); outcome=json.dumps({'delta':d['changed_pixels'],'terminal':terminal},separators=(',',':')); key=tuple(map(str,sequence))
        self.rules.setdefault(key,RuleStats()).observe(outcome,d['changed_count'],terminal); self._tested.add(key); self._terminal_found|=terminal
        rec={'kind':'Transition','state_before':d['before_hash'],'actions':list(sequence),'state_after':d['after_hash'],'delta':d,'terminal':terminal,'entities_before':connected_components(before),'entities_after':connected_components(after)}; self.transitions.append(rec)
        hid='rule:'+sha256(('|'.join(key)).encode()).hexdigest()[:12]
        self.graphene_records += [{'kind':'Observation','id':'obs:'+sha256(json.dumps(rec,sort_keys=True).encode()).hexdigest()[:12],'payload':rec},{'kind':'Hypothesis','id':hid,'statement':f"sequence {key} predicts observed transition class",'support':self.rules[key].confidence,'samples':self.rules[key].n,'status':'supported' if self.rules[key].confidence>=self.saturation_confidence else 'contested'}]
        return rec
    def information_score(self,sequence):
        key=tuple(map(str,sequence)); s=self.rules.get(key)
        if s: return 1/(1+s.n)+(1-s.confidence)+min(1.0,s.entropy)+s.interesting-0.03*len(sequence)
        parent=self.rules.get(key[:-1]) if len(key)>1 else None
        return 1.0+(parent.interesting if parent else .5)-0.03*len(sequence)
    def _expand_depth(self,depth):
        if depth<=1 or depth in self._frontiers: return
        parents=[p for p in self._frontiers.get(depth-1,[]) if p in self.rules]
        changed=sorted((p for p in parents if self.rules[p].max_changed>0),key=lambda p:self.rules[p].interesting,reverse=True)
        silent=sorted((p for p in parents if self.rules[p].max_changed==0),key=lambda p:(self.rules[p].n,p))
        keep=changed[:self.beam_width]
        for p in silent[:self.silent_quota]:
            if p not in keep: keep.append(p)
        self._frontiers[depth]=[p+(str(a),) for p in keep for a in self.actions]
    def candidate_sequences(self):
        for depth in range(1,self.max_depth+1):
            self._expand_depth(depth); untested=[p for p in self._frontiers.get(depth,[]) if p not in self._tested]
            if untested:
                for p in untested: yield list(p)
                return
        for key,s in self.rules.items():
            if s.n<2 or s.confidence<self.saturation_confidence: yield list(key)
    def next_experiment(self):
        c=list(self.candidate_sequences()); return max(c,key=self.information_score) if c else None
    def crawl(self,reset:Callable[[],Frame],step:Callable[[Action],tuple[Frame,bool]],budget:int=100):
        for _ in range(budget):
            seq=self.next_experiment()
            if seq is None: break
            before=reset(); after=before; terminal=False; prefix=[]
            for a in seq:
                prefix.append(a); after,terminal=step(a)
                if terminal: break
            self.record(before,prefix,after,terminal)
            if terminal: break
        return {'transitions':len(self.transitions),'rules':len(self.rules),'remaining':self.next_experiment() is not None,'graphene_records':len(self.graphene_records),'terminal_found':self._terminal_found,'deepest_tested':max((len(k) for k in self._tested),default=0)}
