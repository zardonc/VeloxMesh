import {spawnSync} from 'node:child_process';
import {appendFileSync, existsSync, mkdirSync, readFileSync, writeFileSync} from 'node:fs';

const root = process.env.SHIP_RESULTS_ROOT;
if (!root || !process.env.SHIP_BINARY || !process.env.SHIP_RUNNER) throw Error('Explicit artifact/binary/runner paths required');
mkdirSync(root, {recursive:true});
const helper = '.planning/phases/29-semantic-cache-latency-hardening/measurements/scenario-validation-20261003/run.mjs';
const modes = ['TestLiveDirectLoad', 'TestLiveCacheLoadOff', 'TestLiveCacheLoadOnWithoutMemo', 'TestLiveCacheLoadOnMemo'];
let failures = 0;
function run(options) {
  if (existsSync(`${root}/PAUSE`)) process.exit(2);
  const args = [helper, options.provider ?? 'LOCAL', options.model ?? 'qwen2.5-0.5b-instruct',
    options.tests.join(','), options.label, String(options.count ?? 100), String(options.interval ?? 125), String(options.concurrency ?? 4)];
  const result = spawnSync(process.execPath, args, {env:{...process.env, ...options.env},
    encoding:'utf8', timeout:Math.max(3, options.tests.length)*80000+30000});
  appendFileSync(`${root}/progress.jsonl`, JSON.stringify({completed:new Date().toISOString(), ...options,
    status:result.status, error:result.error?.message, stdout:result.stdout, stderr:result.stderr})+'\n');
  console.log(JSON.stringify({label:options.label, status:result.status}));
  if (result.status !== 0) failures++;
  return result.status;
}

const stage = process.argv[2];
if (stage === 'safety') {
  run({label:'cache-policies', tests:['TestLiveExactCachePolicy', 'TestLiveDisabledCachePolicy', 'TestLiveImplicitCachePolicy']});
  run({label:'provider-protection', tests:['TestLiveProviderDeadlineProtection', 'TestLiveProviderConcurrencyRecovery', 'TestLiveProviderCancellationRecovery', 'TestLiveProviderSharedResource']});
  run({label:'embedding-isolation', tests:['TestLiveEmbeddingModelSwitch'], env:{PHASE29_SECOND_EMBEDDING_MODEL:'text-embedding-nomic-embed-text-v1.5'}});
} else if (stage === 'performance') {
  for (const scenario of ['low-hit', 'pure-miss']) for (let block=1;block<=6;block++) {
    run({label:`${scenario}-${block}`, tests:block%2?modes:[...modes].reverse(),
      env:{PHASE29_PURE_MISS:scenario==='pure-miss'?'true':'false'}});
  }
  run({label:'hit-comparison', tests:['TestLiveCacheHitSettlementLoad','TestLiveEmbeddingMemoHitBilling','TestLiveCacheHitBaseline']});
} else if (stage === 'capacity') {
  for (const concurrency of [1,2,4,8]) run({label:`capacity-c${concurrency}`, tests:['TestLiveCacheBypassLoad','TestLiveMissBurst'], concurrency});
} else if (stage === 'protocol') {
  run({label:'protected-local-stream', tests:['TestLiveStreamSettlement','TestLiveStreamCancellation','TestLiveBufferedStream','TestLiveFusionStream'],
    env:{PHASE29_PROVIDER_PROTECTION:JSON.stringify({'sans-primary':{first_byte_timeout:'5s',first_content_timeout:'5s',stream_idle_timeout:'5s',total_timeout:'30s',max_inflight:4}})}});
} else if (stage === 'regression') {
  run({label:'backend-regression', tests:[], count:16, interval:1000, concurrency:2});
} else if (stage === 'exact-safety') {
  run({label:'exact-safety-regression', tests:['TestLiveExactCacheSafetyRegression','TestLiveExactCacheVersionIsolation']});
  const progress = readFileSync(`${root}/progress.jsonl`, 'utf8').trim().split('\n').map(line=>JSON.parse(line));
  const selection = [];
  for (const item of progress.filter(item=>/^(low-hit|pure-miss)-[1-6]$/.test(item.label))) {
    const label = item.status === 0 ? item.label : `${item.label}-recheck`;
    if (item.status !== 0) {
      const {tests, provider, model, count, interval, concurrency, env} = item;
      run({tests, provider, model, count, interval, concurrency, env, label});
    }
    selection.push({original:item.label, selected:label, original_status:item.status});
  }
  writeFileSync(`${root}/block-selection.json`, JSON.stringify({policy:'one recheck after recorded failure; original evidence retained; availability failures remain reportable', blocks:selection},null,2)+'\n');
  if (progress.some(item=>item.label==='provider-protection' && item.status!==0)) {
    run({label:'provider-protection-recheck', tests:['TestLiveProviderDeadlineProtection','TestLiveProviderConcurrencyRecovery','TestLiveProviderCancellationRecovery','TestLiveProviderSharedResource']});
  }
  const additionalBinary = '.tmp/policy-execution-20261003/app-linux-additional.test';
  const build = spawnSync('go', ['test','-tags','phase29preflight','-c','-o',additionalBinary,'./internal/app'],
    {env:{...process.env,GOOS:'linux',GOARCH:'amd64',CGO_ENABLED:'0'},encoding:'utf8',timeout:60000});
  writeFileSync(`${root}/additional-build.log`, `${build.stdout??''}${build.stderr??''}`);
  if (build.status !== 0) throw Error('Additional lifecycle test compilation failed');
  run({label:'additional-guards', tests:['TestLiveProviderOverallTimeout','TestLiveProviderRegistryCapacity'],
    env:{SHIP_BINARY:additionalBinary}});
} else if (stage === 'final-checks') {
  run({label:'provider-protection-final', tests:['TestLiveProviderDeadlineProtection','TestLiveProviderConcurrencyRecovery',
    'TestLiveProviderCancellationRecovery','TestLiveProviderSharedResource','TestLiveProviderOverallTimeout','TestLiveProviderRegistryCapacity']});
  run({label:'protected-stream-final', tests:['TestLiveStreamSettlement','TestLiveStreamCancellation','TestLiveBufferedStream','TestLiveFusionStream'],
    env:{PHASE29_PROVIDER_PROTECTION:JSON.stringify({'sans-primary':{first_byte_timeout:'5s',first_content_timeout:'5s',stream_idle_timeout:'5s',total_timeout:'30s',max_inflight:4}})}});
  run({label:'backend-regression-final', tests:[], count:16, interval:1000, concurrency:2});
} else throw Error('Unknown verification stage');
process.exitCode = failures ? 1 : 0;
