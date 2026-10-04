import {spawnSync} from 'node:child_process';
import {appendFileSync} from 'node:fs';

const root = process.env.SHIP_RESULTS_ROOT;
if (!root || !process.env.SHIP_BINARY || !process.env.SHIP_RUNNER) throw Error('explicit replay paths required');
const helper = '.planning/phases/29-semantic-cache-latency-hardening/measurements/scenario-validation-20261003/run.mjs';
let failedBatches = 0;
function run(options) {
  const args = [helper, options.provider ?? 'LOCAL', options.model ?? 'qwen2.5-0.5b-instruct', options.tests.join(','), options.label,
    String(options.count ?? 100), String(options.interval ?? 125), String(options.concurrency ?? 4)];
  const started = new Date().toISOString();
  const child = spawnSync(process.execPath, args, {env:{...process.env, ...options.env}, encoding:'utf8', timeout:options.tests.length*80000+40000});
  appendFileSync(`${root}/final-progress.jsonl`, JSON.stringify({started, completed:new Date().toISOString(), ...options,
    status:child.status, output:child.stdout, stderr:child.stderr, error:child.error?.message})+'\n');
  console.log(JSON.stringify({label:options.label, provider:options.provider??'LOCAL', status:child.status}));
  if (child.status !== 0) failedBatches++;
  return child.status === 0;
}
const stable = ['TestLiveDirectLoad','TestLiveCacheLoadOff','TestLiveCacheLoadOn'];
for (let block=1; block<=6; block++) run({label:`final-stable-${block}`,tests:block%2?stable:[...stable].reverse()});
run({label:'final-scenarios',tests:['TestLiveCacheBypassLoad','TestLiveMissBurst'],concurrency:8});
run({label:'final-hit',tests:['TestLiveCacheHitSettlementLoad','TestLiveCacheHitBaseline']});
run({label:'final-chain',tests:['TestLiveAuthentication','TestLiveCacheMissEmbeddingReuse','TestLiveEmbeddingNetworkDeadline','TestLiveSearchNetworkDeadline','TestLiveQdrantCrossProcess','TestLiveQdrantErrorClassification']});
run({label:'final-embedding',tests:['TestLiveEmbeddingStress'],env:{PHASE29_EMBEDDING_PARALLEL_WARMUP:'16'}});
const candidates = [['OR','inclusionai/ling-3.0-flash-sante:free'],['OR','nvidia/nemotron-3.5-lightning:free'],['GPT','gpt-6-luna']];
const available = [];
for (const [provider, model] of candidates) {
  if (run({label:'final-availability',provider,model,tests:['TestLiveModelAvailabilityBudget']})) available.push([provider,model]);
  else run({label:'final-availability-direct',provider,model,tests:['TestLiveRawAvailabilityBudget']});
}
const protocols = ['TestLiveStreamSettlement','TestLiveStreamCancellation','TestLiveBufferedStream','TestLiveFusionStream',
  'TestLiveToolRequired','TestLiveToolNamed','TestLiveToolAuto','TestLiveToolOmitted','TestLiveToolNone','TestLiveToolStreaming'];
for (let index=0; index<available.length; index++) {
  const [provider,model] = available[index];
  run({label:'final-protocol',provider,model,tests:protocols.filter((_,i)=>i%available.length===index)});
}
if (available.length) {
  const [provider, model] = available[0];
  run({label:'final-business',provider,model,tests:['TestLiveDirectBusinessFAQResponses','TestLiveBusinessFAQResponses']});
}
process.exitCode = failedBatches ? 1 : 0;
