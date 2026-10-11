import {spawnSync} from 'node:child_process';
import {appendFileSync, mkdirSync} from 'node:fs';

const root = process.env.SHIP_RESULTS_ROOT;
const model = process.env.SHIP_LOCAL_MODEL;
const hostBase = process.env.DIRECT_EMBEDDING_BASE_URL;
const embeddingModel = process.env.PHASE29_MODEL;
if (!root || !model || !hostBase || !embeddingModel) throw Error('explicit suite inputs required');
mkdirSync(root, {recursive:true});

function run(options) {
  const tests = options.tests.join(',');
  const started = new Date().toISOString();
  const child = spawnSync(process.execPath, [`${root}/run.mjs`, options.provider ?? 'LOCAL', options.model ?? model,
    tests, options.label, String(options.count ?? 100), String(options.interval ?? 125), String(options.concurrency ?? 4)],
    {env:{...process.env, ...options.env}, encoding:'utf8', timeout:options.tests.length*80000+30000});
  appendFileSync(`${root}/suite-progress.jsonl`, JSON.stringify({started, completed:new Date().toISOString(),
    label:options.label, status:child.status, error:child.error?.message, output:child.stdout, stderr:child.stderr})+'\n');
  console.log(JSON.stringify({label:options.label,status:child.status}));
}

function host(label, concurrency) {
  const child = spawnSync(process.execPath,[`${root}/local-embedding.mjs`,hostBase,embeddingModel,model,`${root}/${label}`,'100',String(concurrency)],
    {encoding:'utf8', timeout:120000});
  appendFileSync(`${root}/suite-progress.jsonl`, JSON.stringify({label,status:child.status, output:child.stdout, stderr:child.stderr})+'\n');
  console.log(JSON.stringify({label,status:child.status}));
}

const stableTests = ['TestLiveDirectLoad','TestLiveCacheLoadOff','TestLiveCacheLoadOn','TestLiveCacheLoadOffBare','TestLiveCacheLoadOnBare'];
for (let block=1; block<=6; block++) run({label:`stable-${block}`,tests:block%2 ? stableTests : [...stableTests].reverse()});
for (const [rps,interval] of [[4,250],[8,125],[12,83],[16,63]]) run({label:`rate-${rps}`,tests:['TestLiveCacheLoadOff','TestLiveCacheLoadOn'],interval});
run({label:'burst-c8',tests:['TestLiveMissBurst'],concurrency:8});
for (const budget of ['10ms','20ms','50ms']) run({label:`read-${budget}`,tests:['TestLiveCacheLoadOn'],env:{PHASE29_READ_TIMEOUT:budget}});
for (let block=1; block<=3; block++) {
  host(`host-before-${block}`,4);
  run({label:`embedding-pair-${block}`,tests:['TestLiveEmbeddingStress','TestLiveEmbeddingChatCoload']});
  host(`host-after-${block}`,4);
}
host('host-c8',8);
run({label:'embedding-c8',tests:['TestLiveEmbeddingStress'],concurrency:8});
run({label:'semantic-scores',tests:['TestLiveEmbeddingSemanticScores']});
run({label:'nomic-validation',tests:['TestLiveEmbeddingStress','TestLiveSemanticDiagnosticPositive','TestLiveSemanticDiagnosticNegative','TestLiveEmbeddingSemanticScores'],
  env:{PHASE29_MODEL:process.env.PHASE29_SECOND_EMBEDDING_MODEL}});
run({label:'sans-full-request',provider:'SANS',model:process.env.SHIP_SANS_MODEL, count:1,concurrency:1,
  tests:['TestLiveDirectLoad','TestLiveCacheLoadOff','TestLiveCacheLoadOn']});
run({label:'profile-c8',tests:['TestLiveMissBurst'],concurrency:8,env:{SHIP_PROFILE:'true',SHIP_TELEMETRY:'true'}});
