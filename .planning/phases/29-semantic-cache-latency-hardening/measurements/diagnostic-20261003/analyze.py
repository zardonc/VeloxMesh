from pathlib import Path
import json, math, statistics
root=Path('.planning/phases/29-semantic-cache-latency-hardening/measurements/diagnostic-20261003')
def pct(values,p):
    return sorted(values)[max(0,math.ceil(len(values)*p)-1)] if values else None
def distribution(values):
    return {'n':len(values),'p50_ms':pct(values,.5),'p95_ms':pct(values,.95),'p99_ms':pct(values,.99),'mean_ms':statistics.mean(values) if values else None}
def read(path):
    result=[]
    for line in path.read_text().splitlines():
        if 'SAMPLE ' in line:
            try:result.append(json.loads(line.split('SAMPLE ',1)[1]))
            except json.JSONDecodeError: pass
    return result
runs=[]
for path in sorted(root.rglob('Test*.log')):
    records=read(path); clients=[x for x in records if x.get('type')=='client' and 'request_id' in x];stages=[x for x in records if x.get('type')=='stage']
    run={'log':path.relative_to(root).as_posix(),'test':path.stem,'passed':'--- PASS:' in path.read_text(),'fallback_vector':'failed to initialize Qdrant' in path.read_text()}
    run['failures']=[x.strip() for x in path.read_text().splitlines() if any(k in x for k in ['error_code=','native upstream error','upstream status=','real completion returned','deadline exceeded','--- FAIL:','no such file'])]
    if clients:
        run.update({'count':len(clients),'ok':sum(x['ok'] for x in clients),'hits':sum(x['hit'] for x in clients),'latency':distribution([x['elapsed_ms'] for x in clients if x['ok']]),'dispatch_lag':distribution([max(0,x['start_ms']-x.get('planned_ms',0)) for x in clients]),'peak_concurrency':max(x['concurrent'] for x in clients)})
        meta=next((x for x in records if x.get('type')=='metadata'),{})
        run['actual_rps']=len(clients)*1000/meta['elapsed_ms'] if 'elapsed_ms' in meta else None
        grouped={c['request_id']:[s for s in stages if s['request_id']==c['request_id']] for c in clients}
        measured={};residual=[];cacheplus=[];upstream_wait=[];pool=[];body=[]
        for c in clients:
            ss=grouped[c['request_id']];byname={}
            for s in ss:byname.setdefault(s['name'],[]).append(s)
            for name,segments in byname.items():
                if not name.startswith('net_'):measured.setdefault(name,[]).append(sum(s['elapsed_ms'] for s in segments))
            def total(name):return sum(s['elapsed_ms'] for s in byname.get(name,[]))
            if 'http_handler' in byname:
                residual.append(total('http_handler')-total('provider_complete')-total('cache_read'))
                cacheplus.append(total('http_handler')-total('provider_complete'))
            if 'net_upstream/first_byte' in byname and 'net_upstream/request_written' in byname:
                upstream_wait.append(total('net_upstream/first_byte')-total('net_upstream/request_written'))
            if 'upstream_http' in byname and 'upstream_headers' in byname:body.append(total('upstream_http')-total('upstream_headers'))
            conns=byname.get('net_upstream/got_connection_new',[])+byname.get('net_upstream/got_connection_reused',[])
            if conns:pool.append(sum(s['elapsed_ms'] for s in conns))
        run['stages']={k:distribution(v) for k,v in measured.items()}
        run['gateway_excluding_primary_and_cache']=distribution(residual)
        run['gateway_including_cache_excluding_primary']=distribution(cacheplus)
        run['upstream_wait_after_request_write']=distribution(upstream_wait)
        run['upstream_connection_acquire']=distribution(pool)
        run['upstream_body_consumption']=distribution(body)
        run['per_model']={m:{'count':sum(c['model']==m for c in clients),'ok':sum(c['model']==m and c['ok'] for c in clients),'latency':distribution([c['elapsed_ms'] for c in clients if c['model']==m and c['ok']])} for m in set(c['model'] for c in clients)}
    embedding=[x for x in records if x.get('type')=='embedding_client']
    if embedding:run['embedding']={'count':len(embedding),'ok':sum(x['ok'] for x in embedding),'dimensions':sorted(set(x['dimension'] for x in embedding)),'latency':distribution([x['elapsed_ms'] for x in embedding if x['ok']])}
    run['outcomes']={}
    for sample in records:
        if sample.get('type')=='outcome':run['outcomes'][sample['name']]=run['outcomes'].get(sample['name'],0)+1
    runs.append(run)
summary={'quantile_method':'nearest_rank','runs':runs}
(root/'summary.json').write_text(json.dumps(summary,indent=2,ensure_ascii=False))
for run in runs:
    if 'count' in run:print(run['log'],run['ok'],run['count'],'p95',run['latency']['p95_ms'],'lag',run['dispatch_lag']['p95_ms'],'fallback',run['fallback_vector'])
print('runs',len(runs),'failures',sum(not r['passed'] for r in runs))
