"""Diagnostic cache-path comparison; keep invalid fixtures and drift explicit."""
import importlib.util
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
REPO = next(path for path in ROOT.parents if (path / 'go.mod').exists())
sys.path.insert(0, str(REPO / 'scripts'))
from phase29_metrics import comparison_gate, stats, summarize_windows

spec = importlib.util.spec_from_file_location('prior', ROOT.parent / 'policy-execution-20261003/evaluate.py')
prior = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prior)


def window(folder, test, expected_reuse=None):
    path = ROOT / folder / (test + '.log')
    rows, records = prior.samples(path, diagnostic=True), prior.records(path)
    if len(rows) != 100 or any(not row['ok'] for row in rows):
        raise ValueError(f'Incomplete or failed formal window: {path}')
    if f'--- PASS: {test} (' not in path.read_text():
        raise ValueError(f'Missing real PASS marker: {path}')
    profiles = [row for row in records if row['type'] == 'effective_cache_profile']
    profile = profiles[0] if profiles else {}
    costs = {name: stats([row['stages'].get(name, 0) for row in rows])
             for name in ('scope_readiness', 'cache_read', 'embedding_http', 'vector_search', 'repo_read', 'provider_complete')}
    counts = {name: sum(name in row['stages'] for row in rows) for name in costs}
    valid = expected_reuse is None or profile.get('reuse_mode') == expected_reuse
    if expected_reuse == 'exact' and any(counts[name] for name in ('embedding_http', 'vector_search', 'scope_readiness')):
        valid = False
    return {'summary': summarize_windows([rows]), 'effective_profile': profile,
            'protocol_valid': valid, 'stage_costs': costs, 'stage_counts': counts,
            'first_three': rows[:3]}


def compare(before, after, candidate):
    first, last = (value['summary']['latency']['p95_ms'] for value in (before, after))
    drift = 100 * (last / first - 1)
    return {'diagnostic_only': True, 'acceptance_approved': False,
            'off_before_p95_ms': first, 'off_after_p95_ms': last,
            'off_drift_pct': drift, 'drift_screen_passed': abs(drift) <= 10,
            'conservative_anchor': 'slower of both bracketing off windows',
            **comparison_gate(candidate['summary']['latency']['p95_ms'], max(first, last))}


def main():
    initial = 'path-comparison'
    corrected = 'exact-comparison-corrected'
    windows = {
        'initial_off': window(initial, 'TestLiveControlledOff'),
        'mislabelled_exact_actually_semantic': window(initial, 'TestLiveControlledExactMiss', 'exact'),
        'semantic_miss': window(initial, 'TestLiveControlledOn'),
        'semantic_hit': window(initial, 'TestLiveCacheHitSettlementLoad'),
        'initial_off_after': window(initial, 'TestLiveControlledOffAfter'),
        'corrected_off': window(corrected, 'TestLiveControlledOff'),
        'exact_miss': window(corrected, 'TestLiveControlledExactMiss', 'exact'),
        'corrected_off_after': window(corrected, 'TestLiveControlledOffAfter'),
    }
    if not windows['exact_miss']['protocol_valid']:
        raise ValueError('Corrected exact window has wrong effective profile or semantic stages')
    windows['semantic_hit']['protocol_valid'] = False
    windows['semantic_hit']['schedule_note'] = 'C4 burst; functional hit/settlement evidence, not matched 8 RPS'
    result = {'windows': windows, 'comparisons': {
        'semantic_miss': compare(windows['initial_off'], windows['initial_off_after'], windows['semantic_miss']),
        'exact_miss': compare(windows['corrected_off'], windows['corrected_off_after'], windows['exact_miss']),
    }, 'limits': ['single fixed-order block per mode; no release approval',
                 'mislabelled exact window excluded', 'semantic hit is an unscheduled C4 burst with a different seed workload',
                 'no new synchronized host/GPU capture; previous hardware result remains bounded',
                 'all early formal samples retained; seeds/warmup kept separately']}
    (ROOT / 'path-evaluation.json').write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps({'comparisons': result['comparisons'], 'windows': {
        label: {'n': value['summary']['n'], 'failed': value['summary']['failed'],
                'summary': value['summary'],
                'stage_counts': value['stage_counts']} for label, value in windows.items()}}, indent=2))


if __name__ == '__main__':
    main()
