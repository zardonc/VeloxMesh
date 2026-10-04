import {readFileSync, writeFileSync} from 'node:fs';
import {createHash} from 'node:crypto';

const [source, output] = process.argv.slice(2);
if (!source || !output) throw Error('explicit source/output required');
const content = readFileSync(source,'utf8');
let wallTime = '';
const tasks = new Map();
for (const line of content.split(/\r?\n/)) {
  const wall = line.match(/^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\]/);
  if (wall) wallTime = wall[1];
  const event = line.match(/(\d+)\.(\d+)\.(\d+)\.(\d+) I slot (launch_slot_|print_timing): id\s+(\d+) \| task\s+(\d+) \| (.*)/);
  if (!event) continue;
  const elapsed = (Number(event[1])*60+Number(event[2]))*1000+Number(event[3])+Number(event[4])/1000;
  const id = event[7], item = tasks.get(id) ?? {task_id:id, slot:Number(event[6])};
  if (event[5] === 'launch_slot_') {
    Object.assign(item,{wall_time:wallTime,start_ms:elapsed});
  } else {
    item.end_ms = elapsed;
    const duration = event[8].match(/(prompt eval time|eval time|total time)\s*=\s*([\d.]+) ms/);
    if (duration) item[duration[1].replaceAll(' ','_')+'_ms'] = Number(duration[2]);
  }
  tasks.set(id,item);
}
function summarize(start,end) {
  const selected = [...tasks.values()].filter(x=>x.wall_time>=start && x.wall_time<=end && x.end_ms>=x.start_ms);
  const events = selected.flatMap(x=>[{at:x.start_ms,delta:1},{at:x.end_ms,delta:-1}]).sort((a,b)=>a.at-b.at || a.delta-b.delta);
  let active=0, peak=0;
  for (const event of events) { active+=event.delta; peak=Math.max(peak,active); }
  const stats = field=>{const values=selected.filter(x=>Number.isFinite(x[field])).map(x=>x[field]).sort((a,b)=>a-b);
    return values.length?{n:values.length,p50_ms:values[Math.ceil(values.length*.5)-1],p95_ms:values[Math.ceil(values.length*.95)-1],max_ms:values.at(-1)}:null;};
  return {start,end,count:selected.length,slots:[...new Set(selected.map(x=>x.slot))],peak_observed_slots:peak,
    prompt:stats('prompt_eval_time_ms'),decode:stats('eval_time_ms'),total:stats('total_time_ms'),tasks:selected};
}
const result = {source_file:source.split('/').at(-1),source_sha256:createHash('sha256').update(content).digest('hex'),
  clock_validation:'first task launch-to-print 61.098ms vs logged prompt+decode 61.00ms; elapsed clock interpretation consistent',
  limits:'wall-time second precision; windows include warmups/boundary tasks; no gateway-request-ID correlation, no embedding inference timing',
  windows:[summarize('2026-10-03 21:37:47','2026-10-03 21:38:02'),summarize('2026-10-03 21:41:00','2026-10-03 21:41:59')]};
writeFileSync(output,JSON.stringify(result,null,2)+'\n');
console.log(JSON.stringify(result.windows.map(({tasks,...x})=>x)));
