import {spawnSync} from 'node:child_process';
import {existsSync, mkdirSync, writeFileSync} from 'node:fs';

// Local-only, cache-off experiment. Credentials remain in environment/stdin.
const root = '.planning/phases/29-semantic-cache-latency-hardening/measurements/primary-warmup-investigation-20261004';
if (existsSync(`${root}/batch.log`)) throw Error('Evidence already exists; use a fresh run directory');
mkdirSync(root, {recursive: true});
const env = {...process.env, SHIP_ARTIFACTS: root,
  SHIP_BINARY: '.tmp/primary-warmup-investigation-20261004/app-linux.test', SHIP_PROVIDER: 'LOCAL',
  SHIP_LOCAL_BASE_URL: 'http://127.0.0.1:11234/v1', SHIP_LOCAL_MODEL: 'qwen2.5-0.5b-instruct',
  SHIP_LOCAL_API_KEY: 'local-test', PHASE29_MODEL: 'text-embedding-embedder_collection',
  PHASE29_EMBEDDING_BASE_URL: 'http://127.0.0.1:11234/v1', PHASE29_EMBEDDING_API_KEY: 'local-test',
  PHASE29_READ_TIMEOUT: '100ms', PHASE29_READ_CONCURRENCY: '4', PHASE29_WRITE_WORKERS: '2',
  PHASE29_QUEUE_CAPACITY: '32', PHASE29_WRITE_TIMEOUT: '2s', PHASE29_SHUTDOWN_GRACE: '1s',
  PHASE29_COUNT: '100', PHASE29_INTERVAL_MS: '125', PHASE29_CLIENT_CONCURRENCY: '4',
  PHASE29_PURE_MISS: 'true', PHASE29_REUSE_MODE: 'semantic', PHASE29_MEMO_CAPACITY: '0',
  PHASE29_MEMO_TTL: '', PHASE29_INPUT_PREFIX: '', PHASE29_THRESHOLD: '.92',
  PHASE29_PROVIDER_PROTECTION: '', PHASE29_EXTRA_PROVIDERS: '', PHASE29_MIXED_MODELS: '', SHIP_TELEMETRY: 'true'};
const start = new Date().toISOString();
const result = spawnSync('.tmp/primary-warmup-investigation-20261004/runner.exe', ['warmup'],
  {env, encoding: 'utf8', timeout: 1000000});
writeFileSync(`${root}/batch.log`, (result.stdout ?? '') + (result.stderr ?? ''));
writeFileSync(`${root}/run-result.json`, JSON.stringify({start, end: new Date().toISOString(),
  status: result.status, error: result.error?.message}, null, 2)+'\n');
console.log(result.stdout);
if (result.stderr) console.error(result.stderr);
process.exitCode = result.status ?? 1;
