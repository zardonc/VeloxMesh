import {spawnSync} from 'node:child_process';
import {appendFileSync, mkdirSync, writeFileSync} from 'node:fs';

const root = process.env.SHIP_RESULTS_ROOT;
if (!root || !process.env.SHIP_RUNNER || !process.env.SHIP_BINARY) throw Error('Explicit paths required');
mkdirSync(root,{recursive:true});
const helper = '.planning/phases/29-semantic-cache-latency-hardening/measurements/scenario-validation-20261003/run.mjs';
const plans = [
  {label:'low-rate',count:30,interval:1000},
  {label:'target-rate-1',count:100,interval:125},
  {label:'target-rate-2',count:100,interval:125},
];
const tests = ['TestLiveDirectLoad','TestLiveControlledOff','TestLiveControlledOn','TestLiveControlledMemo','TestLiveControlledOffAfter'];
writeFileSync(`${root}/test-plan.json`,JSON.stringify({started:new Date().toISOString(),plans,tests,
  purpose:'Hardware diagnostic; pure misses, same images and fresh volume; retain early requests',
  release_gate:1.05,retry:false,client_ceiling:4},null,2)+'\n');
function run(label, selected, count, interval) {
  const started = new Date().toISOString();
  const result = spawnSync(process.execPath,[helper,'LOCAL','qwen2.5-0.5b-instruct',selected.join(','),label,String(count),String(interval),'4'],
    {env:process.env,encoding:'utf8',timeout:selected.length*80000+30000});
  appendFileSync(`${root}/progress.jsonl`,JSON.stringify({started,ended:new Date().toISOString(),label,status:result.status,
    error:result.error?.message,stdout:result.stdout,stderr:result.stderr})+'\n');
  console.log(JSON.stringify({label,status:result.status}));
  if(result.status!==0) throw Error(`Diagnostic ${label} failed; no replacement window`);
}
run('availability',['TestLiveModelAvailability'],1,1000);
for(const plan of plans) run(plan.label,tests,plan.count,plan.interval);
run('components',['TestLiveRedisCapacity','TestLiveQdrantCapacity','TestLiveSQLiteCapacity','TestLivePostgresCapacity'],1,1000);
