#!/usr/bin/env python3
from dialectical_world_crawler import DialecticalWorldCrawler

class TinyWorld:
    def __init__(self): self.reset()
    def reset(self): self.x=0; self.key=False; self.door=False; return self.frame()
    def frame(self):
        g=[[0]*4 for _ in range(4)]; g[3][self.x]=1; g[1][1]=2 if not self.key else 0; g[0][3]=4 if self.door else 3; return g
    def step(self,a):
        if a=='R': self.x=min(3,self.x+1)
        elif a=='L': self.x=max(0,self.x-1)
        elif a=='P' and self.x==1: self.key=True
        elif a=='O' and self.key and self.x==3: self.door=True
        return self.frame(), self.door

def main():
    w=TinyWorld(); c=DialecticalWorldCrawler(['R','L','P','O'],max_depth=6,saturation_confidence=.95,beam_width=20,silent_quota=6)
    summary=c.crawl(w.reset,w.step,budget=220)
    assert summary['transitions']>0
    assert summary['deepest_tested']>=5, 'crawler must be willing to deepen beyond immediate effects'
    assert any(r['terminal'] for r in c.transitions), 'crawler must discover a delayed terminal interaction'
    assert any(len(r['actions'])>=2 and r['delta']['changed_count']>0 for r in c.transitions), 'must test multi-step interactions'
    assert any(x['kind']=='Hypothesis' for x in c.graphene_records)
    terminal=[r['actions'] for r in c.transitions if r['terminal']]
    assert terminal and len(terminal[0])>=5, 'hidden mechanic should require a genuinely deep sequence'
    # Silence must not be equated with irrelevance: at least one no-change prefix should be expanded.
    silent_expanded=False
    tested={tuple(map(str,r['actions'])):r for r in c.transitions}
    for seq,r in tested.items():
        if r['delta']['changed_count']==0 and any(len(k)>len(seq) and k[:len(seq)]==seq for k in tested):
            silent_expanded=True; break
    assert silent_expanded, 'crawler must preserve curiosity about silent prefixes'
    print({'pass':True,'summary':summary,'terminal_sequences':terminal[:5],'silent_expanded':silent_expanded})
if __name__=='__main__': main()
