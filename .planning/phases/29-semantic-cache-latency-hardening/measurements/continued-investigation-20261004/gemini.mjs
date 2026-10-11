import {spawnSync} from 'node:child_process';
import {appendFileSync,existsSync,mkdirSync,readFileSync,writeFileSync} from 'node:fs';

const root = '.planning/phases/29-semantic-cache-latency-hardening/measurements/continued-investigation-20261004/gemini-v2';
if(existsSync(`${root}/progress.jsonl`)) throw Error('Evidence exists; refusing overwrite');
mkdirSync(root,{recursive:true});
const cfg=Object.fromEntries(readFileSync('.env.local','utf8').split(/\r?\n/).filter(line=>/^\w+=/.test(line)).map(line=>{
  const index=line.indexOf('=');return [line.slice(0,index),line.slice(index+1).replace(/^['"]|['"]$/g,'')];
}));
const model=cfg.GEM_PRIMARY_DEFAULT_MODEL;
if(!model || !cfg.GEM_PRIMARY_API_KEY || !cfg.GEM_BASE_URL) throw Error('Gemini preflight configuration incomplete');
const plan=[...Array.from({length:3},(_,index)=>({label:`pair-${index+1}`,tests:['TestLiveGeminiNativeTimeline','TestLiveGeminiToolTimeline']})),
  {label:'cancellation',tests:['TestLiveGeminiCancelBeforeComplete','TestLiveGeminiCompleteBeforeCancel']}];
writeFileSync(`${root}/test-plan.json`,JSON.stringify({started:new Date().toISOString(),model,plan,retry:false,
  scope:'same actual Gemini SDK/provider; private metadata hashed; cancellation gate controls delivery, not physical provider completion'},null,2)+'\n');
let failed=false;
for(const window of plan) {
  if(failed) break;
  const started=new Date().toISOString();
  const result=spawnSync(process.execPath,['.planning/phases/29-semantic-cache-latency-hardening/measurements/scenario-validation-20261003/run.mjs',
    'GEM',model,window.tests.join(','),window.label,'1','1000','1'],{env:{...process.env,SHIP_RESULTS_ROOT:root,
    SHIP_RUNNER:'.tmp/continued-investigation-20261004/runner.exe',SHIP_BINARY:'.tmp/continued-investigation-20261004/app-gemini.test',
    SHIP_TELEMETRY:'true'},encoding:'utf8',timeout:190000});
  appendFileSync(`${root}/progress.jsonl`,JSON.stringify({started,ended:new Date().toISOString(),label:window.label,status:result.status,
    error:result.error?.message,stdout:result.stdout,stderr:result.stderr})+'\n');
  console.log(JSON.stringify({label:window.label,status:result.status}));
  failed=result.status!==0;
}
process.exitCode=failed?1:0;
