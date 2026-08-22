#!/usr/bin/env python3
from dialectical_world_crawler import DialecticalWorldCrawler

class TinyWorld:
    def __init__(self): self.reset()
    def reset(self):
        self.x=0; self.key=False; self.door=False; return self.frame()
    def frame(self):
        # 4x4 visual-only state: player=1, key=2, door=3/open=4
        g=[[0]*4 for _ in range(4)]; g[3][self.x]=1; g[1][1]=2 if not self.key else 0; g[0][3]=4 if self.door else 3; return g
    def step(self,a):
        if a=='R': self.x=min(3,self.x+1)
        elif a=='L': self.x=max(0,self.x-1)
        elif a=='P' and self.x==1: self.key=True
        elif a=='O' and self.key and self.x==3: self.door=True
        return self.frame(), self.door

def main():
    w=TinyWorld(); c=DialecticalWorldCrawler(['R','L','P','O'],max_depth=3,saturation_confidence=.95)
    summary=c.crawl(w.reset,w.step,budget=140)
    assert summary['transitions']>0
    assert any(r['terminal'] for r in c.transitions), 'crawler must discover a terminal interaction'
    assert any(len(r['actions'])>=2 and r['delta']['changed_count']>0 for r in c.transitions), 'must test multi-step interactions'
    assert any(x['kind']=='Hypothesis' for x in c.graphene_records)
    # Deterministic repeated rules should become confident and pruneable.
    confident=[s for s in c.rules.values() if s.n>=2 and s.confidence>=.95]
    assert confident, 'must consolidate deterministic transition hypotheses'
    print({'pass':True,'summary':summary,'terminal_sequences':[r['actions'] for r in c.transitions if r['terminal']][:5],'confident_rules':len(confident)})
if __name__=='__main__': main()
