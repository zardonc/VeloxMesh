from pathlib import Path
import json
import re
import sys

root = Path(__file__).resolve().parent
trace = Path(sys.argv[1])
pattern = re.compile(r'Range(Begin|End) Time=(\d+) Name="(stop-the-world [^"]+)" Scope=(\S+)')
starts, events = {}, []
for line in trace.open():
    match = pattern.search(line)
    if match is None:
        continue
    kind, stamp, name, scope = match.groups()
    key = (name, scope)
    if kind == 'Begin':
        starts[key] = int(stamp)
    elif key in starts:
        events.append(dict(name=name,duration_ms=(int(stamp)-starts.pop(key))/1e6))
if starts or not events:
    raise SystemExit('Incomplete trace accounting')
folder = root/'profile-c8'/'LOCAL-qwen2.5-0.5b-instruct'
cpu = lambda name: [int(x) for x in (folder/f'TestLiveMissBurst-vm-{name}.log').read_text().splitlines()[1].split()[1:9]]
deltas = [a-b for a,b in zip(cpu('after'),cpu('before'))]
total = sum(deltas)
summary = dict(stw_count=len(events),stw_max_ms=max(r['duration_ms'] for r in events),
               stw_total_ms=sum(r['duration_ms'] for r in events),stw_events=events,
               vm_cpu_deltas=deltas,vm_busy_percent=100*(total-deltas[3]-deltas[4])/total,
               vm_steal_percent=100*deltas[7]/total)
(root/'profile-summary.json').write_text(json.dumps(summary,indent=2))
print(json.dumps({k:v for k,v in summary.items() if k!='stw_events'},indent=2))
