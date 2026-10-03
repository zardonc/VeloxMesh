from pathlib import Path
from collections import defaultdict, Counter
import json
import math
import random
import statistics

ROOT = Path(__file__).resolve().parent

def quantile(values, percentile=.95):
    return sorted(values)[math.ceil(len(values)*percentile)-1] if values else None

def distribution(values):
    if not values:
        return None
    return dict(n=len(values), p50_ms=quantile(values,.5), p95_ms=quantile(values),
                p99_ms=quantile(values,.99), max_ms=max(values), mean_ms=statistics.mean(values))

def records(path):
    return [json.loads(line.split('SAMPLE ',1)[1]) for line in path.read_text().splitlines() if 'SAMPLE ' in line]

def peak(stages):
    events = sorted([(s['start_ms'],1) for s in stages] +
                    [(s['start_ms']+s['elapsed_ms'],-1) for s in stages])
    active, highest = 0, 0
    for _, change in events:
        active += change
        highest = max(highest,active)
    return highest

def request_metrics(client, stages):
    groups = defaultdict(list)
    for stage in stages:
        groups[stage['name']].append(stage)
    total = lambda name: sum(s['elapsed_ms'] for s in groups[name])
    output = {'id':client['request_id'], 'hit':client['hit'], 'latency':client['elapsed_ms']}
    if groups['http_handler']:
        output['gateway'] = total('http_handler')-total('provider_complete')-total('cache_read')
        output['gateway_plus_cache'] = total('http_handler')-total('provider_complete')
    if groups['cache_read']:
        start = groups['cache_read'][0]['start_ms']
        end = start+total('cache_read')
        overlap = lambda s: max(0,min(end,s['start_ms']+s['elapsed_ms'])-max(start,s['start_ms']))
        output['cache_local'] = total('cache_read')-sum(overlap(s) for name in ['lookup_embedding','vector_search'] for s in groups[name])
        output['cache_read'] = total('cache_read')
    if groups['net_upstream/first_byte'] and groups['net_upstream/request_written']:
        output['provider_wait_after_write'] = total('net_upstream/first_byte')-total('net_upstream/request_written')
    return output

def summarize_log(path):
    raw, content = records(path), path.read_text()
    clients = [r for r in raw if r['type']=='client' and r.get('request_id')]
    stages = [r for r in raw if r['type']=='stage']
    by_id = defaultdict(list)
    for stage in stages:
        by_id[stage['request_id']].append(stage)
    metrics = [request_metrics(c,by_id[c['request_id']]) for c in clients]
    values = lambda key: [r[key] for r in metrics if key in r]
    result = dict(log=path.relative_to(ROOT).as_posix(), passed=f'--- PASS: {path.stem} (' in content,
                  clients=len(clients), failed=sum(not c['ok'] for c in clients), hits=sum(c['hit'] for c in clients))
    result['latency'] = distribution([c['elapsed_ms'] for c in clients if c['ok']])
    result['metrics'] = {key:distribution(values(key)) for key in ['gateway','gateway_plus_cache','cache_local','cache_read','provider_wait_after_write']}
    result['actual_handler_concurrency'] = peak([s for c in clients for s in by_id[c['request_id']] if s['name']=='http_handler'])
    result['actual_client_concurrency'] = max((c['concurrent'] for c in clients),default=0)
    result['outcomes'] = dict(Counter(r['name'] for r in raw if r['type']=='outcome'))
    result['errors'] = [line.strip() for line in content.splitlines() if '--- FAIL:' in line or 'semantic diagnostic failures=' in line]
    meta = next((r for r in raw if r['type']=='metadata'),None)
    if meta:
        result['actual_rps'] = len(clients)*1000/meta['elapsed_ms']
        result['dispatch_lag'] = distribution([max(0,c['start_ms']-c['planned_ms']) for c in clients])
    result['special'] = [r for r in raw if r['type'] in ['semantic_diagnostic','embedding_model_switch','network_recovery','partial_write','cross_process_collection','hit_settlement','business_faq','direct_business_faq','real_dimension_mismatch','redis_post_provider_delay']]
    result['embedding'] = summarize_embedding(raw)
    result['stages'] = {name:distribution([s['elapsed_ms'] for s in stages if s['name']==name]) for name in set(s['name'] for s in stages) if not name.startswith('net_')}
    result['request_metrics'] = metrics
    return result

def summarize_embedding(raw):
    samples = [r for r in raw if r['type']=='embedding_client']
    if not samples:
        return None
    return dict(count=len(samples), failed=sum(not r['ok'] for r in samples),
                dimensions=sorted(set(r['dimension'] for r in samples)), latency=distribution([r['elapsed_ms'] for r in samples]),
                chat_failed=sum(not r['ok'] for r in raw if r['type']=='coload_chat'))

def stable_summary(runs):
    blocks = []
    for block in range(1,7):
        cells = {Path(r['log']).stem:r for r in runs if r['log'].startswith(f'stable-{block}/')}
        if len(cells)!=5:
            continue
        off, on = cells['TestLiveCacheLoadOff'], cells['TestLiveCacheLoadOn']
        blocks.append(dict(block=block, ratio=on['latency']['p95_ms']/off['latency']['p95_ms'],
                           cells={k:{key:v for key,v in value.items() if key!='request_metrics'} for k,value in cells.items()}))
    cohorts = {key:[r for r in runs if r['log'].startswith('stable-') and Path(r['log']).stem==key] for key in
               ['TestLiveDirectLoad','TestLiveCacheLoadOff','TestLiveCacheLoadOn','TestLiveCacheLoadOffBare','TestLiveCacheLoadOnBare']}
    pooled = {key:distribution([m['latency'] for r in cohort for m in r['request_metrics']]) for key,cohort in cohorts.items()}
    ci = bootstrap_ratio(cohorts) if len(blocks)==6 else None
    return dict(blocks=blocks, pooled=pooled, block_bootstrap_ratio_ci95=ci)

def bootstrap_ratio(cohorts):
    rng, ratios = random.Random(2903), []
    off, on = cohorts['TestLiveCacheLoadOff'], cohorts['TestLiveCacheLoadOn']
    for _ in range(2000):
        sampled_off, sampled_on = [], []
        for _ in range(6):
            index = rng.randrange(6)
            sampled_off.extend(rng.choices([r['latency'] for r in off[index]['request_metrics']],k=100))
            sampled_on.extend(rng.choices([r['latency'] for r in on[index]['request_metrics']],k=100))
        ratios.append(quantile(sampled_on)/quantile(sampled_off))
    return [quantile(ratios,.025),quantile(ratios,.975)]

def local_embedding_summary():
    result = []
    for path in sorted(ROOT.glob('host-*/local-embedding.json')):
        data = json.loads(path.read_text())
        for window in data['windows']:
            result.append(dict(label=path.parent.name, co_load=window['co_load'], concurrency=window['concurrency'],
                               dimension=data['dimension'], latency=distribution([r['elapsed_ms'] for r in window['records']]),
                               failed=sum(not r['ok'] for r in window['records']),chat_failed=sum(not r['ok'] for r in window['chats'])))
    return result

def semantic_scores():
    result = []
    for path in ROOT.rglob('TestLiveEmbeddingSemanticScores.log'):
        samples = [r for r in records(path) if r['type']=='embedding_semantic_score']
        if not samples:
            continue
        positive = [r['cosine'] for r in samples if r['positive']]
        negative = [r['cosine'] for r in samples if not r['positive']]
        result.append(dict(log=path.relative_to(ROOT).as_posix(),embedding_model=samples[0]['embedding_model'],
                           positive_min=min(positive),positive_max=max(positive),negative_max=max(negative),
                           positive_above_092=sum(s>=.92 for s in positive),negative_above_092=sum(s>=.92 for s in negative),
                           positive_above_negative_max=sum(s>max(negative) for s in positive)))
    return result

def diagnostic_pass(run):
    if run['failed'] or not run['request_metrics']:
        return None
    gateway, local = run['metrics']['gateway'], run['metrics']['cache_local']
    if gateway is None:
        return None
    return dict(gateway_p95=gateway['p95_ms']<=10, gateway_p99=gateway['p99_ms']<=15,
                cache_local_p95=local['p95_ms']<=2 if local else None)

def main():
    runs = [summarize_log(path) for path in sorted(ROOT.rglob('Test*.log')) if '-vm-' not in path.name]
    for run in runs:
        run['diagnostic_budget'] = diagnostic_pass(run)
    summary = dict(quantile_method='nearest_rank',bootstrap_seed=2903,bootstrap_replicates=2000,
                   runs=runs,stable=stable_summary(runs),local_embedding=local_embedding_summary(),semantic_scores=semantic_scores())
    (ROOT/'summary.json').write_text(json.dumps(summary,indent=2,ensure_ascii=False))
    print(json.dumps(dict(tests=len(runs),failed=[r['log'] for r in runs if not r['passed']],stable=summary['stable']['pooled'],
                         embedding=summary['local_embedding'],semantic_scores=summary['semantic_scores']),indent=2))

if __name__=='__main__':
    main()
