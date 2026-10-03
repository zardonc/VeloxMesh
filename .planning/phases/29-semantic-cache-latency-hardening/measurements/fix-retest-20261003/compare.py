from pathlib import Path
import json

root = Path(__file__).resolve().parent

def records(path):
    return [json.loads(line.split('SAMPLE ', 1)[1]) for line in path.read_text().splitlines() if 'SAMPLE ' in line]

def inspect(folder, prefix='stable-block'):
    calls, misses, hits, fallback, attempts, written = 0, 0, 0, 0, 0, 0
    outliers, outcomes = [], {}
    for path in sorted(folder.glob(f'{prefix}-*/LOCAL-*/TestLiveCacheLoadOn*.log')):
        data = records(path)
        stages = [r for r in data if r['type'] == 'stage']
        for c in (r for r in data if r['type'] == 'client'):
            own = [s for s in stages if s['request_id'] == c['request_id']]
            total = lambda name: sum(s['elapsed_ms'] for s in own if s['name'] == name)
            calls += sum(s['name'] == 'embedding_http' for s in own)
            fallback += sum(s['name'] == 'store_embedding' for s in own)
            attempts += sum(s['name'] in ['lookup_embedding', 'store_embedding'] for s in own)
            written += sum(s['name'] == 'net_embedding/request_written' for s in own)
            hits += int(c['hit'])
            misses += int(not c['hit'])
            residual = total('http_handler') - total('cache_read') - total('provider_complete')
            if residual > 15:
                outliers.append({'file': str(path.relative_to(folder)), 'request_id': c['request_id'], 'residual_ms': residual,
                                 'stages': {s['name']: total(s['name']) for s in own if not s['name'].startswith('net_')}})
        for r in data:
            if r['type'] == 'outcome':
                outcomes[r['name']] = outcomes.get(r['name'], 0) + 1
    return {'misses': misses, 'hits': hits, 'completed_embedding_http_calls': calls, 'embedding_invocations': attempts,
            'embedding_requests_written': written, 'background_embedding_fallbacks': fallback,
            'outlier_count': len(outliers), 'outliers': sorted(outliers, key=lambda x: -x['residual_ms']), 'outcomes_including_warmups': outcomes}

before = json.loads((root.parent / 'diagnostic-20261003/stable-summary.json').read_text())
after = json.loads((root / 'verified-block-summary.json').read_text())
comparison = {'before': {'pooled': before['pooled'], 'requests': inspect(root.parent / 'diagnostic-20261003')},
              'after': {'pooled': after['pooled'], 'requests': inspect(root, 'verified-block')},
              'gate_passed': after['performance_gate_passed'],
              'on_p95_change_percent': 100 * (after['pooled']['on']['p95_ms'] / before['pooled']['on']['p95_ms'] - 1),
              'stages_before': before['stages_cache_on'], 'stages_after': after['stages_cache_on']}
(root / 'comparison.json').write_text(json.dumps(comparison, indent=2))
print(json.dumps({k: v for k, v in comparison.items() if not k.startswith('stages_')}, indent=2))
