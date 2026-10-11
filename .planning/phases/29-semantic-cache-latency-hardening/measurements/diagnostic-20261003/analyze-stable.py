from pathlib import Path
import json,math,random,statistics
root=Path('.planning/phases/29-semantic-cache-latency-hardening/measurements/diagnostic-20261003')
def records(p):
 return [json.loads(l.split('SAMPLE ',1)[1]) for l in p.read_text().splitlines() if 'SAMPLE ' in l]
def q(v,p=.95):return sorted(v)[math.ceil(len(v)*p)-1]
def d(v):return {'n':len(v),'p50_ms':q(v,.5),'p95_ms':q(v),'p99_ms':q(v,.99),'mean_ms':statistics.mean(v)} if v else None
blocks=[];all_off=[];all_on=[];stage={};residual=[];plus=[];embed_headers=[];embedding_after_write=[];embedding_after_headers=[]
for b in range(1,4):
 folder=root/f'stable-block-{b}'/'LOCAL-qwen2.5-0.5b-instruct'
 cells={name:records(folder/f'{name}.log') for name in ['TestLiveDirectLoad','TestLiveCacheLoadOff','TestLiveCacheLoadOn','TestLiveCacheLoadOnRepeat','TestLiveCacheLoadOffRepeat']}
 def clients(name):return [r for r in cells[name] if r['type']=='client']
 off=[r['elapsed_ms'] for name in ['TestLiveCacheLoadOff','TestLiveCacheLoadOffRepeat'] for r in clients(name)]
 on=[r['elapsed_ms'] for name in ['TestLiveCacheLoadOn','TestLiveCacheLoadOnRepeat'] for r in clients(name)]
 all_off.extend(off);all_on.extend(on)
 block={'block':b,'off':d(off),'on':d(on),'ratio':q(on)/q(off),'cells':{k:d([r['elapsed_ms'] for r in clients(k)]) for k in cells},'max_dispatch_p95_ms':max(q([max(0,r['start_ms']-r['planned_ms']) for r in clients(k)]) for k in cells)}
 blocks.append(block)
 for name in ['TestLiveCacheLoadOn','TestLiveCacheLoadOnRepeat']:
  recs=cells[name]; cs=clients(name); ss=[r for r in recs if r['type']=='stage']
  for c in cs:
   by={}
   for s in ss:
    if s['request_id']==c['request_id']:by.setdefault(s['name'],[]).append(s)
   total=lambda name:sum(x['elapsed_ms'] for x in by.get(name,[]))
   for k,v in by.items():
    if not k.startswith('net_'):stage.setdefault(k,[]).append(sum(x['elapsed_ms'] for x in v))
   residual.append(total('http_handler')-total('cache_read')-total('provider_complete'))
   plus.append(total('http_handler')-total('provider_complete'))
   for lookup in by.get('cache_read',[]):
    lo,hi=lookup['start_ms'],lookup['start_ms']+lookup['elapsed_ms']
    foreground=lambda name:[s for s in by.get(name,[]) if lo<=s['start_ms']<=hi]
    first,write=foreground('net_embedding/first_byte'),foreground('net_embedding/request_written')
    heads,http=foreground('embedding_headers'),foreground('embedding_http')
    if first and write:embedding_after_write.append(first[0]['elapsed_ms']-write[0]['elapsed_ms'])
    if heads:embed_headers.append(heads[0]['elapsed_ms'])
    if http and heads:embedding_after_headers.append(http[0]['elapsed_ms']-heads[0]['elapsed_ms'])
# Three run blocks provide only limited independent environmental observations.
rng=random.Random(29); boot=[]
for _ in range(2000):
 bo=[];bn=[]
 for _ in range(3):
  i=rng.randrange(3);folder=root/f'stable-block-{i+1}'/'LOCAL-qwen2.5-0.5b-instruct'
  # Cache per-block values extracted once below rather than independent pooling.
  bo.extend(all_off[i*200:(i+1)*200]);bn.extend(all_on[i*200:(i+1)*200])
 boot.append(q(bn)/q(bo))
summary={'blocks':blocks,'pooled':{'off':d(all_off),'on':d(all_on),'ratio':q(all_on)/q(all_off)},'gate_limit':1.05,'performance_gate_passed':q(all_on)/q(all_off)<=1.05,'exploratory_block_bootstrap_ratio_ci95':[q(boot,.025),q(boot,.975)],'bootstrap_unit':'entire_ABBA_run_block','bootstrap_seed':29,'bootstrap_replicates':2000,'stages_cache_on':{k:d(v) for k,v in stage.items()},'gateway_without_primary_or_cache':d(residual),'gateway_plus_cache_without_primary':d(plus),'foreground_embedding_headers':d(embed_headers),'foreground_embedding_wait_after_write':d(embedding_after_write),'foreground_embedding_body_consumption':d(embedding_after_headers)}
(root/'stable-summary.json').write_text(json.dumps(summary,indent=2))
print(json.dumps({k:v for k,v in summary.items() if k not in ['stages_cache_on']},indent=2));print('stage P95',{k:v['p95_ms'] for k,v in summary['stages_cache_on'].items()})
