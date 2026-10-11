import {readFileSync, readdirSync, writeFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {join, relative} from 'node:path';

const root = '.planning/phases/29-semantic-cache-latency-hardening/measurements/availability-investigation-20261004';
const normalized = path => path.replaceAll('\\', '/');
function files(directory) {
  return readdirSync(directory, {withFileTypes: true}).flatMap(entry => entry.isDirectory() ? files(join(directory, entry.name)) : [join(directory, entry.name)]);
}
function testRecord(path) {
  const content = readFileSync(path, 'utf8'), name = path.match(/(Test\w+)\.log$/)[1];
  const clients = content.split(/\r?\n/).filter(line => line.includes('SAMPLE ')).map(line => JSON.parse(line.split('SAMPLE ')[1])).filter(row => row.type === 'client');
  const status = content.includes(`--- PASS: ${name} (`) ? 'PASS' : content.includes(`--- FAIL: ${name} (`) ? 'FAIL' : 'INCOMPLETE';
  const local = normalized(relative(root, path));
  return {path: local, name, status, expected_red: local.startsWith('baseline-network/') && name === 'TestLiveRoutingHealthDependencyError' || local.startsWith('routes-corrected-red/') && name === 'TestLiveHealthDependencyRoutes',
    fixture_failure: local.startsWith('routes-red/'), clients: clients.length, failed_clients: clients.filter(row => !row.ok).length};
}
const tests = files(root).filter(path => /[/\\]Test\w+\.log$/.test(path)).map(testRecord);
const regressionPath = `${root}/final-backend/LOCAL-qwen2.5-0.5b-instruct/full-backend.jsonl`;
const rows = readFileSync(regressionPath, 'utf8').trim().split('\n').map(JSON.parse);
const direct = JSON.parse(readFileSync(`${root}/direct-replay/upstream-replay-summary.json`, 'utf8'));
const workload = JSON.parse(readFileSync(`${root}/workload-readout.json`, 'utf8'));
const onLog = readFileSync(`${root}/workload/LOCAL-qwen2.5-0.5b-instruct/TestLiveCacheLoadOnWithoutMemo.log`, 'utf8');
const lookupErrors = onLog.split(/\r?\n/).filter(line => line.includes('"msg":"semantic cache operation failed"') && line.includes('"operation":"lookup"')).length;
const revision = spawnSync('git', ['rev-parse', 'HEAD'], {encoding: 'utf8'});
if (revision.status !== 0) throw Error('Revision read failed');
const summary = {base_revision: revision.stdout.trim(), uncommitted_source: true, classification_fix: 'fail-closed health_state_unavailable, not latency/availability resolution', tests,
  final_backend: {path: normalized(relative(root, regressionPath)), passed: rows.filter(row => row.Action === 'pass' && row.Test).length,
    top_level_passed: rows.filter(row => row.Action === 'pass' && row.Test && !row.Test.includes('/')).length,
    packages_passed: rows.filter(row => row.Action === 'pass' && !row.Test).length,
    skipped_tests: rows.filter(row => row.Action === 'skip' && row.Test).map(row => ({package: row.Package, test: row.Test})),
    packages_without_tests: rows.filter(row => row.Action === 'skip' && !row.Test).map(row => row.Package),
    failed: rows.filter(row => row.Action === 'fail')},
  direct: {normal_requests: direct.summaries.reduce((n, window) => n + window.requests, 0), failures: direct.summaries.reduce((n, window) => n + window.failed, 0), intentional_cancellations: direct.cancellations.length},
  workload: {blocks: workload.blocks, gate: workload.point_gate, windows: Object.fromEntries(Object.entries(workload.summaries).map(([mode, window]) => [mode,
    {requests: window.n, failures: window.failed, hits: window.hits, actual_rps: window.actual_rps, actual_max_concurrency: window.actual_max_concurrency, latency: window.latency, application: window.application}])),
    initial_qdrant_lookup_errors: lookupErrors, release_validated: false},
  cleanup_log: 'workload/LOCAL-qwen2.5-0.5b-instruct/runner.log',
  pending: ['natural Redis read stalls and steady-state host attribution', 'Qdrant scope initialization and collection growth inventory', 'upstream 400 raw engine cause/correlation', 'balanced six-block performance confirmation', 'business-approved semantic evaluation and isolated model cold state'],
  provenance: {baseline_sources: 'baseline/internal/', observer_executed_source: 'baseline/redis-observation-executed.txt', observer_replay_source: 'redis-observation.go', previous_evidence_unchanged: '../policy-execution-20261003'}};
if (summary.final_backend.failed.length || tests.filter(row => row.path.startsWith('green/') || row.path.startsWith('final-backend/') || row.path.startsWith('workload/')).some(row => row.status !== 'PASS')) throw Error('Final evidence is not PASS');
writeFileSync(`${root}/summary.json`, JSON.stringify(summary, null, 2) + '\n');
const inventory = spawnSync('git', ['ls-files', '--modified', '--others', '--exclude-standard'], {encoding: 'utf8'});
if (inventory.status !== 0) throw Error('Source inventory failed');
const sourcePaths = inventory.stdout.trim().split('\n').filter(path => /^(internal|tests|scripts|docs)\//.test(path) || /\.(py|mjs|go|md)$/.test(path));
const binaryPaths = ['app-baseline.test', 'app-routes-red.test', 'app-routes-corrected-red.test', 'app-green.test', 'redis-observation-executed.exe', 'redis-observation-replay.exe'].map(name => `.tmp/availability-investigation-20261004/${name}`);
binaryPaths.push('.tmp/policy-execution-20261003/runner-verified.exe');
const hash = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const evidence = files(root).filter(path => !path.endsWith('manifest.json'));
writeFileSync(`${root}/manifest.json`, JSON.stringify({base_revision: summary.base_revision, source_state: 'uncommitted; current post-measurement documentation snapshot',
  sources: Object.fromEntries(sourcePaths.map(path => [path, hash(path)])), binaries: Object.fromEntries(binaryPaths.map(path => [path, hash(path)])),
  evidence: Object.fromEntries(evidence.map(path => [normalized(relative(root, path)), hash(path)]))}, null, 2) + '\n');
console.log(JSON.stringify({backend: summary.final_backend, green: tests.filter(row => row.path.startsWith('green/')).length, direct: summary.direct, records: tests.length}, null, 2));
