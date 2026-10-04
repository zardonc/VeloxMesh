import {spawnSync} from 'node:child_process';
import {appendFileSync, existsSync, mkdirSync, writeFileSync} from 'node:fs';

// Preserve every block, including failed blocks; no replacement or selective retry.
const root = process.env.SHIP_RESULTS_ROOT;
if (!root || !process.env.SHIP_BINARY || !process.env.SHIP_RUNNER) throw Error('Explicit paths required');
if (process.env.PHASE29_REUSE_MODE !== 'semantic') throw Error('Controlled comparison requires reuse_mode=semantic');
mkdirSync(root, {recursive: true});
const helper = '.planning/phases/29-semantic-cache-latency-hardening/measurements/scenario-validation-20261003/run.mjs';
const orders = [
  ['TestLiveControlledOff', 'TestLiveControlledOn', 'TestLiveControlledMemo', 'TestLiveControlledOffAfter'],
  ['TestLiveControlledOff', 'TestLiveControlledMemo', 'TestLiveControlledOn', 'TestLiveControlledOffAfter'],
];
const plan = {created: new Date().toISOString(), protocol: 'Six bracketing blocks per scenario; alternate on/memo order; no retries',
  primary_model: 'qwen2.5-0.5b-instruct', embedding_model: 'text-embedding-embedder_collection',
  count_per_window: 100, interval_ms: 125, client_ceiling: 4, memo_capacity: 128, memo_ttl: '1m',
  cache_reuse_mode: 'semantic', cache_read_timeout: '100ms', health_read_timeout: '50ms (unchanged application setting)',
  seed_write_wait: true, quiet_ms: 2000, windows: []};
for (const scenario of ['low-hit', 'pure-miss']) for (let block = 1; block <= 6; block++) {
  plan.windows.push({label: `${scenario}-${block}`, tests: orders[(block-1)%2], scenario, block});
}
writeFileSync(`${root}/test-plan.json`, JSON.stringify(plan, null, 2)+'\n');
let failures = 0;
function run(label, tests, env = {}, count = 100, interval = 125) {
  if (existsSync(`${root}/PAUSE`)) throw Error('Pause requested; active runner already cleaned up');
  const started = new Date().toISOString();
  const result = spawnSync(process.execPath, [helper, 'LOCAL', plan.primary_model, tests.join(','), label,
    String(count), String(interval), '4'], {env: {...process.env, ...env}, encoding: 'utf8',
    timeout: Math.max(1, tests.length)*80000+30000});
  appendFileSync(`${root}/progress.jsonl`, JSON.stringify({started, completed: new Date().toISOString(), label,
    tests, status: result.status, error: result.error?.message, stdout: result.stdout, stderr: result.stderr})+'\n');
  console.log(JSON.stringify({label, status: result.status}));
  if (result.status !== 0) failures++;
}
run('availability', ['TestLiveModelAvailability']);
run('profile-smoke', orders[0], {PHASE29_PURE_MISS: 'true'}, 1, 125);
if (failures) throw Error('Profile preflight failed; formal matrix not started');
for (const window of plan.windows) run(window.label, window.tests,
  {PHASE29_PURE_MISS: String(window.scenario === 'pure-miss')});
run('guards', ['TestLiveEmbeddingMemoHitBilling', 'TestLiveExactCacheSafetyRegression', 'TestLiveExactCacheVersionIsolation',
  'TestLiveProviderDeadlineProtection', 'TestLiveProviderConcurrencyRecovery', 'TestLiveProviderCancellationRecovery',
  'TestLiveProviderSharedResource', 'TestLiveProviderOverallTimeout', 'TestLiveProviderRegistryCapacity']);
run('protected-stream', ['TestLiveStreamSettlement', 'TestLiveStreamCancellation', 'TestLiveBufferedStream', 'TestLiveFusionStream'],
  {PHASE29_PROVIDER_PROTECTION: JSON.stringify({'sans-primary': {first_byte_timeout: '5s', first_content_timeout: '5s',
    stream_idle_timeout: '5s', total_timeout: '30s', max_inflight: 4}})});
run('backend-regression', [], {}, 16, 1000);
process.exitCode = failures ? 1 : 0;
