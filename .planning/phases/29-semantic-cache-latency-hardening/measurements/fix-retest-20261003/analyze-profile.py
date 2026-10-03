from pathlib import Path
import json, re, subprocess, sys

root = Path(__file__).resolve().parent
label = sys.argv[1] if len(sys.argv) > 1 else 'profiling-final'
folder = root / label / 'LOCAL-qwen2.5-0.5b-instruct'
trace = folder / 'TestLiveCacheLoadOn.trace'
pattern = re.compile(r'Range(Begin|End) Time=(\d+) Name="(stop-the-world [^"]+)" Scope=(\S+)')
starts, events = {}, []
process = subprocess.Popen(['go', 'tool', 'trace', '-d=parsed', str(trace)], stdout=subprocess.PIPE, text=True)
for line in process.stdout:
    match = pattern.search(line)
    if match is None:
        continue
    kind, stamp, name, scope = match.groups()
    key = (name, scope)
    if kind == 'Begin':
        starts[key] = int(stamp)
    elif key in starts:
        events.append({'name': name, 'duration_ms': (int(stamp) - starts.pop(key)) / 1e6})
if process.wait() != 0:
    raise SystemExit('Go execution trace parsing failed')
if starts or not events:
    raise SystemExit('Incomplete STW event accounting')
data = [json.loads(line.split('SAMPLE ', 1)[1]) for line in (folder / 'TestLiveCacheLoadOn.log').read_text().splitlines() if 'SAMPLE ' in line]
stages = [row for row in data if row['type'] == 'stage']
clients = [row for row in data if row['type'] == 'client']
residual = []
for client in clients:
    own = [row for row in stages if row['request_id'] == client['request_id']]
    total = lambda name: sum(row['elapsed_ms'] for row in own if row['name'] == name)
    residual.append(total('http_handler') - total('cache_read') - total('provider_complete'))
summary = {'profiled_requests': len(clients), 'failed': sum(not row['ok'] for row in clients),
           'outliers_above_15ms': sum(ms > 15 for ms in residual), 'residual_max_ms': max(residual),
           'stw_count': len(events), 'stw_max_ms': max(row['duration_ms'] for row in events),
           'stw_total_ms': sum(row['duration_ms'] for row in events), 'stw_events': events}
(root / f'{label}-summary.json').write_text(json.dumps(summary, indent=2))
print(json.dumps({key: value for key, value in summary.items() if key != 'stw_events'}, indent=2))
