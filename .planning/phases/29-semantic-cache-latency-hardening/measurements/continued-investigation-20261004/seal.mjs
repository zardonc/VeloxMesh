import {createHash} from 'node:crypto';
import {readdirSync,readFileSync,writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {join,relative} from 'node:path';

const root='.planning/phases/29-semantic-cache-latency-hardening/measurements/continued-investigation-20261004';
function files(directory){return readdirSync(directory,{withFileTypes:true}).flatMap(entry=>entry.isDirectory()?files(join(directory,entry.name)):[join(directory,entry.name)]);}
const privateConfig=readFileSync('.env.local','utf8').split(/\r?\n/).filter(line=>/^\w+=/.test(line)).map(line=>{
  const index=line.indexOf('=');return [line.slice(0,index),line.slice(index+1).replace(/^['"]|['"]$/g,'')];
}).filter(([key,value])=>/(KEY|PASSWORD|SECRET|TOKEN|DEV_SERVER_PW)/.test(key)&&value.length>=4);
const evidence=files(root).filter(path=>!path.endsWith('manifest.json'));
const hash=path=>createHash('sha256').update(readFileSync(path)).digest('hex');
const sources=['internal/providers/gemini/adapter.go','internal/app/live_gemini_timeline_test.go',
  'internal/app/live_ship_protocol_test.go','internal/app/live_ship_gemini_diagnostic_test.go',
  'tests/integration/redis_hotstate_test.go','.tmp/continued-investigation-20261004/vmctl.go'];
const reports=['.planning/phases/29-semantic-cache-latency-hardening/29-CONTINUED-INVESTIGATION-20261004.md',
  '.planning/phases/29-semantic-cache-latency-hardening/29-VERIFICATION.md'];
for(const path of [...evidence,...sources,...reports]){const content=readFileSync(path,'utf8');for(const[,value]of privateConfig)if(content.includes(value))throw Error(`Credential audit failed: ${path}`);}
const binaries=['app-linux.test','app-gemini.test','runner.exe'].map(file=>`.tmp/continued-investigation-20261004/${file}`);
const revision=spawnSync('git',['rev-parse','HEAD'],{encoding:'utf8'});
if(revision.status!==0)throw Error('Revision read failed');
const backend=readFileSync(`${root}/backend-final/full-backend.jsonl`,'utf8').trim().split('\n').map(JSON.parse);
const failures=backend.filter(row=>row.Action==='fail');
if(failures.length)throw Error('Final backend regression failed');
const terminal=['TestLiveGeminiCancelBeforeComplete','TestLiveGeminiInterruptedBody','TestLiveGeminiCompleteBeforeCancel'];
for(const test of terminal)if(!readFileSync(`${root}/gemini-terminal-green/${test}.log`,'utf8').includes(`--- PASS: ${test} (`))throw Error(`Missing live PASS: ${test}`);
const manifest={created:new Date().toISOString(),head:revision.stdout.trim(),uncommitted:true,production_enabled:false,
  backend:{top_level_passed:backend.filter(row=>row.Action==='pass'&&row.Test&&!row.Test.includes('/')).length,
    failed:failures,skips:backend.filter(row=>row.Action==='skip'&&row.Test).map(row=>({package:row.Package,test:row.Test}))},
  verified_terminal_tests:terminal,source_sha256:Object.fromEntries(sources.map(path=>[path,hash(path)])),
  report_sha256:Object.fromEntries(reports.map(path=>[path,hash(path)])),
  binary_sha256:Object.fromEntries(binaries.map(path=>[path,hash(path)])),
  evidence_sha256:Object.fromEntries(evidence.map(path=>[relative(root,path).replaceAll('\\','/'),hash(path)])),
  limits:['Mislabelled first hardware block retained and excluded from registered comparisons.',
    'First Gemini wrappers failed before upstream I/O; corrected run remains separate.',
    'Gateway Gemini headers timeout remains a failure; no automatic retry.',
    'Historical cancellation red binary hash unavailable; interrupted-body red binary has retained hash.',
    'Low-rate host spike and drift prevent performance acceptance.'],credential_audit:'passed'};
writeFileSync(`${root}/manifest.json`,JSON.stringify(manifest,null,2)+'\n');
console.log(JSON.stringify({backend:manifest.backend,terminal,credential_audit:manifest.credential_audit}));
