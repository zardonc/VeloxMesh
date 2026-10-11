import {spawnSync} from 'node:child_process';
import {appendFileSync, mkdirSync} from 'node:fs';

const root = process.env.SHIP_RESULTS_ROOT;
if (!root || !process.env.SHIP_BINARY || !process.env.SHIP_RUNNER) throw Error('explicit replay paths required');
mkdirSync(root, {recursive:true});
const helper = '.planning/phases/29-semantic-cache-latency-hardening/measurements/scenario-validation-20261003/run.mjs';
let failures = 0;
function run(options) {
  const args = [helper, options.provider ?? 'LOCAL', options.model ?? 'qwen2.5-0.5b-instruct', options.tests.join(','), options.label,
    String(options.count ?? 100), String(options.interval ?? 125), String(options.concurrency ?? 4)];
  const child = spawnSync(process.execPath, args, {env:{...process.env, ...options.env}, encoding:'utf8', timeout:options.tests.length*80000+40000});
  appendFileSync(`${root}/progress.jsonl`, JSON.stringify({completed:new Date().toISOString(), ...options,
    status:child.status, output:child.stdout, stderr:child.stderr, error:child.error?.message})+'\n');
  console.log(JSON.stringify({label:options.label, status:child.status}));
  if (child.status !== 0) failures++;
  return child.status === 0;
}

if (process.argv[2] === 'quality') {
  for (const candidate of [
    {label:'holdout-collection-raw', env:{PHASE29_THRESHOLD:'0.92'}},
    {label:'holdout-nomic-raw', env:{PHASE29_MODEL:'text-embedding-nomic-embed-text-v1.5', PHASE29_THRESHOLD:'0.92'}},
    {label:'holdout-nomic-prefix', env:{PHASE29_MODEL:'text-embedding-nomic-embed-text-v1.5', PHASE29_INPUT_PREFIX:'search_query: ', PHASE29_THRESHOLD:'0.84'}}
  ]) run({...candidate, tests:['TestLiveSemanticIndependentHoldout']});
} else if (process.argv[2] === 'cloud') {
  const cloud = {provider:'GPT', model:'gpt-6-luna'};
  if (run({...cloud, label:'healthy-preflight', tests:['TestLiveRawAvailabilityBudget','TestLiveModelAvailabilityBudget']})) {
    run({...cloud, label:'healthy-protocol', tests:['TestLiveStreamSettlement','TestLiveStreamCancellation','TestLiveBufferedStream',
      'TestLiveFusionStream','TestLiveToolRequired','TestLiveToolNamed','TestLiveToolAuto','TestLiveToolOmitted','TestLiveToolNone','TestLiveToolStreaming']});
    run({...cloud, label:'healthy-business', tests:['TestLiveDirectBusinessFAQResponses','TestLiveBusinessFAQResponses']});
  } else {
    appendFileSync(`${root}/progress.jsonl`, JSON.stringify({type:'skip', provider:'GPT', reason:'preflight failed; see original error; no downstream load dispatched'})+'\n');
  }
} else {
  const stable = ['TestLiveDirectLoad','TestLiveCacheLoadOff','TestLiveCacheLoadOnWithoutMemo','TestLiveCacheLoadOnMemo'];
  for (let block=1; block<=6; block++) run({label:`verified-stable-${block}`, tests:block%2?stable:[...stable].reverse()});
  run({label:'hit-comparison', tests:['TestLiveCacheHitSettlementLoad','TestLiveEmbeddingMemoHitBilling','TestLiveCacheHitBaseline']});
  run({label:'c4-bypass', tests:['TestLiveCacheBypassLoad']});
  run({label:'c8-saturation', tests:['TestLiveCacheBypassLoad','TestLiveMissBurst'], concurrency:8});
  run({label:'embedding-stress', tests:['TestLiveEmbeddingStress'], env:{PHASE29_EMBEDDING_PARALLEL_WARMUP:'16'}});
  run({label:'safety-regression', tests:['TestLiveAuthentication','TestLiveCacheMissEmbeddingReuse','TestLiveEmbeddingNetworkDeadline',
    'TestLiveSearchNetworkDeadline','TestLiveQdrantCrossProcess','TestLiveQdrantErrorClassification','TestLiveCachePendingVisibility']});
  appendFileSync(`${root}/progress.jsonl`, JSON.stringify({type:'skip', providers:['OR','GEM','SANS'],
    reason:'User-authorized resource/quota exclusions; earlier failures preserved'})+'\n');
}
process.exitCode = failures ? 1 : 0;
