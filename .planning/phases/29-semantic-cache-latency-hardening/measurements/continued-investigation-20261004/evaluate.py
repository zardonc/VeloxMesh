"""Hardware screening only; preserve failures and every early request."""
import csv
import importlib.util
import json
import math
import sys
from collections import Counter, defaultdict
from datetime import datetime, timedelta, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parent
DATA = ROOT / (sys.argv[1] if len(sys.argv) > 1 else 'hardware')
REPO = next(path for path in ROOT.parents if (path / 'go.mod').exists())
sys.path.insert(0, str(REPO / 'scripts'))
from phase29_metrics import comparison_gate, stats, summarize_windows

SPEC = importlib.util.spec_from_file_location('prior', ROOT.parent / 'policy-execution-20261003/evaluate.py')
PRIOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PRIOR)
NAMES = {'direct': 'TestLiveDirectLoad', 'off': 'TestLiveControlledOff',
         'on': 'TestLiveControlledOn', 'memo': 'TestLiveControlledMemo',
         'off_after': 'TestLiveControlledOffAfter'}

def utc(value):
    return datetime.fromisoformat(value.replace('Z', '+00:00'))

def distribution(values):
    if not values:
        return None
    ordered = sorted(values)
    return {'n': len(values), 'min': ordered[0], 'max': ordered[-1],
            **{f'p{q}': ordered[math.ceil(len(values) * q / 100)-1] for q in (50, 95, 99)}}

def load_resources():
    host = [json.loads(line) for line in (DATA / 'host-resources.jsonl').read_text().splitlines()]
    vm = []
    for line in (DATA / 'vmstat.jsonl').read_text().splitlines():
        row = json.loads(line)
        fields = row['line'].split()
        if len(fields) < 17 or not all(part.isdigit() for part in fields[:17]):
            continue
        keys = ('running', 'blocked', 'swap_used_kib', 'free_kib', 'buffer_kib', 'cache_kib',
                'swap_in_kibs', 'swap_out_kibs', 'read_kibs', 'write_kibs', 'interrupts', 'switches',
                'user_pct', 'system_pct', 'idle_pct', 'iowait_pct', 'steal_pct')
        vm.append({'utc': row['utc'], **dict(zip(keys, map(int, fields[:17])))})
    gpu, incomplete = [], 0
    for row in csv.DictReader((DATA / 'gpu.csv').read_text().splitlines()):
        row = {key.strip(): value.strip() if value else None for key, value in row.items()}
        if any(value is None for value in row.values()):
            incomplete += 1
            continue
        timestamp = datetime.strptime(row['timestamp'], '%Y/%m/%d %H:%M:%S.%f')
        fields = {'utilization.gpu [%]': 'busy_pct', 'memory.free [MiB]': 'free_mib',
                  'temperature.gpu': 'temperature_c', 'power.draw [W]': 'power_w'}
        gpu.append({'utc': timestamp.replace(tzinfo=timezone(timedelta(hours=-7))).isoformat(),
                    **{label: float(row[key].split()[0]) for key, label in fields.items()}})
    return {'host': host[1:], 'vm': vm[1:], 'gpu': gpu}, incomplete

def resources_for(resources, start, end):
    selected = {name: [row for row in rows if start <= utc(row['utc']) <= end]
                for name, rows in resources.items()}
    host, vm, gpu = (selected[name] for name in ('host', 'vm', 'gpu'))
    if not all(selected.values()):
        raise ValueError('Missing synchronized hardware samples')
    return {'host_cpu_pct': distribution([row['cpu_busy_pct'] for row in host]),
            'host_available_mib': distribution([row['memory_available_bytes']/2**20 for row in host]),
            'vm_cpu_busy_pct': distribution([100-row['idle_pct'] for row in vm]),
            'vm_running': distribution([row['running'] for row in vm]),
            'vm_iowait_pct': distribution([row['iowait_pct'] for row in vm]),
            'vm_steal_pct': distribution([row['steal_pct'] for row in vm]),
            'vm_swap_in_kibs': distribution([row['swap_in_kibs'] for row in vm]),
            'vm_swap_out_kibs': distribution([row['swap_out_kibs'] for row in vm]),
            'gpu_busy_pct': distribution([row['busy_pct'] for row in gpu]),
            'gpu_free_mib': distribution([row['free_mib'] for row in gpu]),
            'gpu_temperature_c': distribution([row['temperature_c'] for row in gpu])}

def psi(path):
    text = path.read_text()
    lines = [line for line in text.splitlines() if line.startswith(('some avg10=', 'full avg10='))]
    totals = [int(line.split('total=')[1]) for line in lines]
    if len(totals) != 6:
        raise ValueError(f'Missing CPU/IO/memory PSI metadata: {path}')
    return dict(zip(('cpu_some', 'cpu_full', 'io_some', 'io_full', 'memory_some', 'memory_full'), totals))

def window(label, mode, resources):
    name = NAMES[mode]
    path = DATA / label / 'LOCAL-qwen2.5-0.5b-instruct' / f'{name}.log'
    rows, raw = PRIOR.samples(path, diagnostic=True), PRIOR.records(path)
    expected = 30 if label == 'low-rate' else 100
    actual = next(row for row in raw if row['type'] == 'metadata')
    if len(rows) != actual['count'] or any(row['hit'] for row in rows):
        raise ValueError(f'Incomplete pure-miss window: {path}')
    summary = summarize_windows([rows])
    times = {row['type']: utc(row['utc']) for row in raw if row['type'] in ('measurement_start', 'measurement_end')}
    before, after = (path.with_name(f'{name}-vm-{phase}.log') for phase in ('before', 'after'))
    if mode == 'direct':
        start = datetime.strptime(before.read_text().splitlines()[0], '%Y-%m-%dT%H:%M:%SZ').replace(tzinfo=timezone.utc)
        end = datetime.strptime(after.read_text().splitlines()[0], '%Y-%m-%dT%H:%M:%SZ').replace(tzinfo=timezone.utc)
    else:
        start, end = times['measurement_start'], times['measurement_end']
    beginning, ending = psi(before), psi(after)
    pressure = {key: ending[key]-value for key, value in beginning.items()}
    return {'summary': summary, 'actual_parameters': actual,
            'protocol_valid': actual['count']==expected and actual['interval_ms']==(1000 if label=='low-rate' else 125),
            'resources': resources_for(resources, start, end),
            'resource_window': 'whole test including warmup' if mode == 'direct' else 'formal requests',
            'psi_stall_us_whole_test': pressure,
            'first_three': [{**row, 'stages': row['stages']} for row in rows[:3]],
            'outcomes': dict(Counter(row['name'] for row in raw if row['type'] == 'outcome'))}

def main():
    resources, incomplete_gpu = load_resources()
    labels = [row['label'] for row in json.loads((DATA / 'test-plan.json').read_text())['plans']]
    windows = {label: {mode: window(label, mode, resources) for mode in NAMES} for label in labels}
    comparisons = {}
    for label, block in windows.items():
        before, after = (block[mode]['summary']['latency']['p95_ms'] for mode in ('off', 'off_after'))
        anchor = max(before, after)
        comparisons[label] = {'acceptance_approved': False, 'diagnostic_only': True,
                              'protocol_valid': all(item['protocol_valid'] for item in block.values()),
                              'off_p95_before': before, 'off_p95_after': after,
                              'off_drift_pct': 100*(after/before-1),
                              'conservative_anchor': 'slower of both bracketing off windows',
                              **{mode: comparison_gate(block[mode]['summary']['latency']['p95_ms'], anchor)
                                 for mode in ('on', 'memo')}}
    components = []
    for path in (DATA / 'components').rglob('Test*.log'):
        if '-vm-' in path.name:
            continue
        groups = defaultdict(list)
        for row in PRIOR.records(path):
            if row['type'] == 'component_client':
                groups[(row['name'], row['concurrent'])].append(row)
        for (name, concurrency), rows in groups.items():
            components.append({'test': path.stem, 'name': name, 'concurrency': concurrency, 'count': len(rows),
                               'failures': sum(not row['ok'] for row in rows),
                               'latency': stats([row['elapsed_ms'] for row in rows])})
    result = {'purpose': 'hardware screen; diagnostic, not a release verdict', 'windows': windows,
              'comparisons': comparisons, 'components': components, 'gpu_incomplete_trailing_rows': incomplete_gpu,
              'limits': ['fixed order; only two target-rate blocks', '30 low-rate requests give coarse tails',
                        'loaded models and synthetic test FAQ', 'no VM resize A/B or hypervisor CPU-ready counter',
                        '1s hardware sampling cannot exclude subsecond contention or aliasing at 1 RPS',
                        'low-rate off-after contains host CPU spikes and exceeds 10 percent drift; no acceptance pass']}
    (ROOT / f'{DATA.name}-evaluation.json').write_text(json.dumps(result, indent=2)+'\n')
    print(json.dumps({'comparisons': comparisons, 'requests': sum(block[mode]['summary']['n'] for block in windows.values() for mode in NAMES),
                      'failures': sum(block[mode]['summary']['failed'] for block in windows.values() for mode in NAMES),
                      'components': components}, indent=2))

if __name__ == '__main__':
    main()
