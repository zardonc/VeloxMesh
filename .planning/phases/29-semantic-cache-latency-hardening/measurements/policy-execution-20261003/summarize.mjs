import {readFileSync, readdirSync, writeFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {join, relative} from 'node:path';

const root = '.planning/phases/29-semantic-cache-latency-hardening/measurements/policy-execution-20261003';
function files(directory) {
  return readdirSync(directory, {withFileTypes:true}).flatMap(entry=>entry.isDirectory()?files(join(directory,entry.name)):[join(directory,entry.name)]);
}
const logs = files(root).filter(path=>/[/\\]Test\w+\.log$/.test(path));
const tests = logs.map(path=>{
  const content = readFileSync(path,'utf8');
  const name = path.match(/(Test\w+)\.log$/)[1];
  const samples = content.split(/\r?\n/).filter(line=>line.includes('SAMPLE ')).map(line=>JSON.parse(line.split('SAMPLE ')[1]));
  const clients = samples.filter(row=>row.type==='client');
  const errors = clients.filter(row=>!row.ok).reduce((counts,row)=>({...counts,[row.error]:(counts[row.error]??0)+1}),{});
  return {path:relative(root,path).replaceAll('\\','/'),name,
    status:content.includes(`--- PASS: ${name} (`)?'PASS':content.includes(`--- FAIL: ${name} (`)?'FAIL':'INCOMPLETE',
    clients:clients.length,failed_clients:clients.filter(row=>!row.ok).length,errors,
    assertions:samples.filter(row=>['exact_safety_regression','exact_version_isolation','provider_protection','provider_overall_timeout',
      'provider_concurrency','provider_cancellation','shared_provider_resource','provider_registry_capacity','embedding_model_switch'].includes(row.type))};
});
const regressionPath = `${root}/final-backend/backend-regression/LOCAL-qwen2.5-0.5b-instruct/full-backend.jsonl`;
const regression = readFileSync(regressionPath,'utf8').trim().split('\n').map(line=>JSON.parse(line));
const summary = {base_revision:spawnSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).stdout.trim(),
  standards_status:'candidate budgets; no phase completion or production approval', tests,
  final_backend:{path:relative(root,regressionPath).replaceAll('\\','/'),
    passed:regression.filter(row=>row.Action==='pass'&&row.Test).length,
    top_level_passed:regression.filter(row=>row.Action==='pass'&&row.Test&&!row.Test.includes('/')).length,
    packages_passed:regression.filter(row=>row.Action==='pass'&&!row.Test).length,
    skipped:regression.filter(row=>row.Action==='skip').map(row=>({package:row.Package,test:row.Test})),
    failed:regression.filter(row=>row.Action==='fail')},
  final_cleanup_log:'final-backend/lifecycle/runner.log',
  pending:['representative business-approved same-answer dataset and recall target','isolated embedding unload/reload cold start',
    'Redis health read timeout availability and VM/host scheduling attribution',
    'upstream 400 and time-aligned LM Studio engine Channel Error attribution','stable scenario performance contracts']};
writeFileSync(`${root}/summary.json`,JSON.stringify(summary,null,2)+'\n');
const hash = path=>createHash('sha256').update(readFileSync(path)).digest('hex');
const changed = spawnSync('git',['ls-files','--modified','--others','--exclude-standard'],{encoding:'utf8'});
if(changed.status!==0)throw Error('Source manifest inventory failed');
const historicalEvaluation = '.planning/phases/29-semantic-cache-latency-hardening/measurements/followup-20261003/evaluation-v2.json';
const sourcePaths = changed.stdout.trim().split('\n').filter(path=>/^(internal|tests|scripts|docs)\//.test(path)||/\.(py|mjs|go|md)$/.test(path)||path===historicalEvaluation);
const binaryPaths = ['app-linux.test','app-linux-final.test','app-linux-additional.test','app-linux-verified.test','runner.exe','runner-final.exe','runner-verified.exe']
  .map(name=>`.tmp/policy-execution-20261003/${name}`);
const evidencePaths = files(root).filter(path=>!path.endsWith('manifest.json'));
writeFileSync(`${root}/manifest.json`,JSON.stringify({base_revision:summary.base_revision,
  sources:Object.fromEntries(sourcePaths.map(path=>[path,hash(path)])),
  binaries:Object.fromEntries(binaryPaths.map(path=>[path,hash(path)])),
  evidence:Object.fromEntries(evidencePaths.map(path=>[relative(root,path).replaceAll('\\','/'),hash(path)]))},null,2)+'\n');
console.log(JSON.stringify({backend:summary.final_backend,live_test_records:tests.length,failed_records:tests.filter(test=>test.status!=='PASS').map(test=>({path:test.path,errors:test.errors}))},null,2));
