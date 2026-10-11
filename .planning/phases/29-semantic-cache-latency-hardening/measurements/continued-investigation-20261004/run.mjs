import {spawn, spawnSync} from 'node:child_process';
import {existsSync, mkdirSync, writeFileSync} from 'node:fs';

const root = '.planning/phases/29-semantic-cache-latency-hardening/measurements/continued-investigation-20261004';
const mode = process.argv[2];
if (!['hardware','hardware-low-rate','gemini-cancellation','gemini-interrupt-red','gemini-terminal-green','local-facts','backend','backend-final','audit'].includes(mode)) throw Error('Explicit supported mode required');
const output = `${root}/${mode}`;
if (existsSync(`${output}/run-result.json`)) throw Error('Evidence exists; refusing overwrite');
mkdirSync(output,{recursive:true});
const env = {...process.env, SHIP_ARTIFACTS:output, SHIP_RESULTS_ROOT:output,
  SHIP_BINARY:'.tmp/continued-investigation-20261004/app-linux.test',
  SHIP_RUNNER:'.tmp/continued-investigation-20261004/runner.exe', SHIP_PROVIDER:'LOCAL',
  SHIP_LOCAL_BASE_URL:'http://127.0.0.1:11234/v1', SHIP_LOCAL_MODEL:'qwen2.5-0.5b-instruct',
  SHIP_LOCAL_API_KEY:'local-test', PHASE29_MODEL:'text-embedding-embedder_collection',
  PHASE29_EMBEDDING_BASE_URL:'http://127.0.0.1:11234/v1', PHASE29_EMBEDDING_API_KEY:'local-test',
  PHASE29_READ_TIMEOUT:'100ms', PHASE29_READ_CONCURRENCY:'4', PHASE29_WRITE_WORKERS:'2',
  PHASE29_QUEUE_CAPACITY:'32', PHASE29_WRITE_TIMEOUT:'2s', PHASE29_SHUTDOWN_GRACE:'1s',
  PHASE29_COUNT:'100', PHASE29_INTERVAL_MS:'125', PHASE29_CLIENT_CONCURRENCY:'4',
  PHASE29_PURE_MISS:'true', PHASE29_REUSE_MODE:'semantic', PHASE29_MEMO_CAPACITY:'0',
  PHASE29_MEMO_TTL:'', PHASE29_INPUT_PREFIX:'', PHASE29_THRESHOLD:'.92', SHIP_TELEMETRY:'true'};
if (mode==='local') env.SHIP_TESTS='TestPhase29LocalGatewayAcceptance,TestLiveModelAvailability,TestLiveExactCacheSafetyRegression,TestLiveExactCacheVersionIsolation';
if (mode==='hardware-low-rate') env.HARDWARE_SCREEN_LOW_RATE='true';
if (mode==='gemini-cancellation') {
  env.SHIP_PROVIDER='GEM';env.SHIP_BINARY='.tmp/continued-investigation-20261004/app-gemini.test';
  env.SHIP_TESTS='TestLiveGeminiCancelBeforeComplete,TestLiveGeminiCompleteBeforeCancel';
}
if (mode==='local-facts') {
  env.SHIP_TESTS='TestLiveDirectBusinessFAQResponses,TestLiveBusinessFAQResponses';
  env.SHIP_BINARY='.tmp/continued-investigation-20261004/app-gemini.test';
}
if (['gemini-interrupt-red','gemini-terminal-green'].includes(mode)) {
  env.SHIP_PROVIDER='GEM';env.SHIP_BINARY='.tmp/continued-investigation-20261004/app-gemini.test';
  env.SHIP_TESTS=mode==='gemini-interrupt-red' ? 'TestLiveGeminiInterruptedBody' : 'TestLiveGeminiCancelBeforeComplete,TestLiveGeminiInterruptedBody,TestLiveGeminiCompleteBeforeCancel';
}
const started = new Date().toISOString();
const gpu = mode.startsWith('hardware') ? spawn('nvidia-smi',['--query-gpu=timestamp,name,memory.total,memory.used,memory.free,utilization.gpu,utilization.memory,temperature.gpu,power.draw,clocks.sm,clocks.mem','--format=csv','-l','1','-f',`${output}/gpu.csv`],{stdio:'ignore'}) : null;
const args = mode.startsWith('hardware') ? ['controlled'] : mode==='backend-final' ? ['backend'] : ['backend','audit'].includes(mode) ? [mode] : [];
const result = spawnSync(env.SHIP_RUNNER,args,{env,encoding:'utf8',timeout:1800000});
if(gpu) gpu.kill();
writeFileSync(`${output}/runner.log`,(result.stdout??'')+(result.stderr??''));
writeFileSync(`${output}/run-result.json`,JSON.stringify({started,ended:new Date().toISOString(),status:result.status,error:result.error?.message},null,2)+'\n');
console.log(result.stdout);
if(result.stderr) console.error(result.stderr);
process.exitCode=result.status??1;
