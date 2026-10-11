import {createHash} from 'node:crypto';
import {readdirSync, readFileSync, statSync, writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {join, relative} from 'node:path';

const root = '.planning/phases/29-semantic-cache-latency-hardening/measurements/further-correction-20261004';
const temp = '.tmp/further-correction-20261004';
const normalize = path => path.replaceAll('\\', '/');
const hash = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const json = path => JSON.parse(readFileSync(path, 'utf8'));
const jsonl = path => readFileSync(path, 'utf8').split(/\r?\n/).filter(Boolean).map(JSON.parse);
const assert = (condition, message) => {if (!condition) throw Error(message);};

function files(directory) {
  return readdirSync(directory, {withFileTypes:true}).flatMap(entry =>
    entry.isDirectory() ? files(join(directory, entry.name)) : [join(directory, entry.name)]);
}

function git(args) {
  const result = spawnSync('git', args, {encoding:'utf8'});
  assert(result.status === 0, 'Git read failed');
  return result.stdout.trim();
}

const changed = [...new Set([
  ...git(['diff', '--name-only']).split('\n'),
  ...git(['ls-files', '--others', '--exclude-standard']).split('\n'),
])].filter(Boolean);
const sources = changed.filter(path => path.startsWith('internal/'));
const reports = changed.filter(path => path.endsWith('.md') && !path.startsWith(`${root}/`));
const evidence = files(root).filter(path => !path.endsWith('manifest.json'));
const privateConfig = readFileSync('.env.local', 'utf8').split(/\r?\n/)
  .filter(line => /^\w+=/.test(line)).map(line => {
    const index = line.indexOf('=');
    return [line.slice(0, index), line.slice(index + 1).replace(/^['"]|['"]$/g, '')];
  }).filter(([key, value]) => /(KEY|PASSWORD|SECRET|TOKEN|DEV_SERVER_PW)/.test(key) && value.length >= 4);
for (const path of [...evidence, ...sources, ...reports]) {
  const content = readFileSync(path, 'utf8');
  assert(!privateConfig.some(([, value]) => content.includes(value)), `Credential audit failed: ${path}`);
}

const backend = jsonl(`${root}/backend-indexed-final/full-backend.jsonl`);
assert(json(`${root}/final-checks.json`).passed, 'Final static checks failed');
const topLevel = row => row.Test && !row.Test.includes('/');
const passed = backend.filter(row => row.Action === 'pass' && topLevel(row)).length;
const failures = backend.filter(row => row.Action === 'fail');
const skips = backend.filter(row => row.Action === 'skip' && topLevel(row))
  .map(row => ({package:row.Package, test:row.Test}));
assert(passed === 599 && failures.length === 0 && skips.length === 2, 'Backend count mismatch');
assert(json(`${root}/backend-indexed-final/run-result.json`).status === 0, 'Backend runner failed');

const live = {
  'gemini-boundaries': ['TestLiveGeminiCancelBeforeComplete', 'TestLiveGeminiInterruptedBody',
    'TestLiveGeminiCompleteBeforeCancel', 'TestLiveGeminiTerminalTailFaults', 'TestLiveGeminiWireComparison'],
  'local-readiness': ['TestLiveScopeReadinessGateway', 'TestPhase29LocalGatewayAcceptance',
    'TestLiveExactCacheSafetyRegression', 'TestLiveExactCacheVersionIsolation'],
  'indexed-readiness-confirmed': ['TestLivePostgresScopeReadiness', 'TestLiveScopeReadinessGateway'],
  'fault-queue-regression': ['TestPhase29LocalEmbeddingFault', 'TestPhase29LocalQueueBurst'],
  'exact-comparison-corrected': ['TestLiveControlledOff', 'TestLiveControlledExactMiss', 'TestLiveControlledOffAfter'],
};
for (const [group, tests] of Object.entries(live)) {
  assert(json(`${root}/${group}/run-result.json`).status === 0, `Runner failed: ${group}`);
  for (const test of tests) {
    const content = readFileSync(`${root}/${group}/${test}.log`, 'utf8');
    assert(content.includes(`--- PASS: ${test} (`), `Missing matching PASS: ${test}`);
    assert(!content.includes(`--- FAIL: ${test}`), `Live failure: ${test}`);
  }
}

const version = json(`${root}/source-version-check.json`);
const binary = `${temp}/app-linux.test`;
const binaryHash = hash(binary);
assert(new Date(version.excluded_old_binary_run.ended) < new Date(version.linux_binary_mtime), 'Old run exclusion mismatch');
assert(new Date(version.confirmed_after_compilation.started) > new Date(version.linux_binary_mtime), 'Confirmed run preceded compile');
assert(statSync(binary).mtime.toISOString() === version.linux_binary_mtime, 'Final binary timestamp mismatch');
assert(readFileSync(`${root}/live-v4-binary.sha256`, 'utf8').startsWith(binaryHash), 'Final binary hash mismatch');
const cleanup = readFileSync(`${root}/cleanup-final.log`, 'utf8');
const cleanupRows = cleanup.split(/\r?\n/).filter(line => line.startsWith('{')).map(JSON.parse);
const collection = cleanupRows.find(row => row.fresh_collection_count !== undefined);
const removed = cleanupRows.find(row => row.removed_owned_binary);
const containers = cleanupRows.filter(row => row.name);
assert(collection.fresh_collection_count === 10 && collection.semantic_count === 9, 'Collection count mismatch');
assert(removed.sha256 === binaryHash, 'Remote binary hash mismatch');
assert(containers.length === 6 && containers.every(row => !row.running && !row.oom && row.restarts === 0), 'Cleanup state mismatch');
assert(!/LISTEN.*:(11234|6379|6333|6334|5432)\s/.test(cleanup), 'Owned service still listening');

const evaluation = json(`${root}/path-evaluation.json`);
assert(evaluation.comparisons.semantic_miss.drift_screen_passed === false, 'Semantic drift classification mismatch');
assert(Object.values(evaluation.comparisons).every(row => row.diagnostic_only && !row.acceptance_approved), 'Performance incorrectly approved');
assert(!evaluation.windows.mislabelled_exact_actually_semantic.protocol_valid, 'Mislabeled window incorrectly accepted');
const shaMap = paths => Object.fromEntries(paths.map(path => [normalize(path), hash(path)]));
const manifest = {
  created:new Date().toISOString(), head:git(['rev-parse', 'HEAD']), uncommitted:true, production_enabled:false,
  backend:{top_level_passed:passed, failed:failures.length, skips}, verified_live_tests:live,
  source_sha256:shaMap(sources), report_sha256:shaMap(reports),
  binary_sha256:shaMap([binary, `${temp}/runner.exe`]),
  evidence_sha256:Object.fromEntries(evidence.map(path => [normalize(relative(root, path)), hash(path)])),
  final_source_version:version, cleanup:{collection, removed, containers}, credential_audit:'passed',
  performance: {acceptance_approved:false, diagnostic_only:true, comparisons:evaluation.comparisons},
  excluded_evidence: {
    'path-comparison/TestLiveControlledExactMiss.log':'Helper overwrote exact mode; retained, excluded.',
    'path-comparison/TestLiveCacheHitSettlementLoad.log':'Functional C4 burst; excluded from matched 8 RPS comparisons.',
    'indexed-readiness':'Run finished before v4 compilation; excluded from index verification.',
    'red.jsonl':'Contains initial fixture compile/schema failures; corrected-fixture red is product evidence.',
    'green-focused.jsonl':'Contains active-scope UTC fixture failure; green-packages is corrected.',
  },
  limits:['One fixed-order Gemini stream pair; canonical JSON equality, connection reuse differs.',
    'Gemini intermittent response-header timeout remains open; no retry or deadline extension.',
    'All path timing samples predate index migrations; semantic baseline drift invalidates acceptance.',
    'Readiness is not persistent collection ownership or garbage collection.',
    'Business seed correctness and semantic negative safety remain release gates.'],
};
writeFileSync(`${root}/manifest.json`, JSON.stringify(manifest, null, 2) + '\n');
console.log(JSON.stringify({backend:manifest.backend, source_files:sources.length,
  evidence_files:evidence.length, credential_audit:manifest.credential_audit, final_binary:binaryHash}));
