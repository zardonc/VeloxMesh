import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync} from 'node:fs';
import {basename, dirname, join, relative, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const repo = resolve(root, '../../../../../');
const runner = '.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner';
const previous = JSON.parse(readFileSync(join(root, '../memory-upgrade-controlled-20261004/manifest.json')));
const currentSources = [...new Set([...Object.keys(previous.source_sha256),
  ...readdirSync(join(repo, runner)).filter(name => name.endsWith('.go')).map(name => `${runner}/${name}`),
  'internal/app/live_primary_warmup_test.go', '.planning/debug/phase29-first-batch.md',
  '.planning/STATE.md', '.planning/phases/29-semantic-cache-latency-hardening/29-VERIFICATION.md',
  '.planning/phases/29-semantic-cache-latency-hardening/29-RECOVERY-INVESTIGATION-20261004.md'])]
  .filter(path => existsSync(join(repo, path)));
const hash = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const sourceDir = join(root, 'source-snapshots');
mkdirSync(sourceDir, {recursive: true});
for (const path of currentSources.filter(path => path.startsWith(runner) || path.endsWith('live_primary_warmup_test.go'))) {
  writeFileSync(join(sourceDir, basename(path)+'.txt'), readFileSync(join(repo, path)));
}
writeFileSync(join(sourceDir, 'storage.go.txt'), readFileSync(join(repo, '.tmp/primary-warmup-investigation-20261004/storage.go')));
const head = execFileSync('git', ['rev-parse', 'HEAD'], {cwd: repo, encoding: 'utf8'}).trim();
writeFileSync(join(root, 'source-head.txt'), head+'\n');
const binaryNames = ['runner.exe', 'runner-idle.exe', 'runner-final.exe', 'app-linux.test',
  'storage.exe', 'storage-readonly.exe', 'storage-layout.exe'];
const binaries = Object.fromEntries(binaryNames.map(name => {
  const path = `.tmp/primary-warmup-investigation-20261004/${name}`;
  return [path, hash(join(repo, path))];
}));
writeFileSync(join(root, 'binaries.sha256'), Object.entries(binaries).map(([path, digest]) => `${digest}  ${path}`).join('\n')+'\n');
function files(directory) {
  return readdirSync(directory, {withFileTypes: true}).flatMap(entry => {
    const path = join(directory, entry.name);
    if (entry.name === '__pycache__' || path === join(root, 'manifest.json')) return [];
    return entry.isDirectory() ? files(path) : [path];
  });
}
const manifest = {created: new Date().toISOString(), head, production_approval: false,
  formal_windows: 0, recovery_failed: true, idle_conditions_completed: 7,
  source_capture: 'Final current sources; actual executed binary hashes preserved. Initial build source snapshots were not all captured before subsequent instrumentation refinements.',
  source_sha256: Object.fromEntries(currentSources.map(path => [path, hash(join(repo, path))])),
  binary_sha256: binaries,
  evidence_sha256: Object.fromEntries(files(root).map(path => [relative(root, path).replaceAll('\\', '/'), hash(path)]))};
writeFileSync(join(root, 'manifest.json'), JSON.stringify(manifest, null, 2)+'\n');
console.log(JSON.stringify({head, evidence: Object.keys(manifest.evidence_sha256).length,
  sources: currentSources.length, binaries: binaryNames.length}));
