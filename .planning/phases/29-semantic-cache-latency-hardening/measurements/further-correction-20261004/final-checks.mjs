import {readdirSync, writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {join} from 'node:path';

const root = '.planning/phases/29-semantic-cache-latency-hardening/measurements/further-correction-20261004';
const env = {...process.env, GOCACHE:join(process.cwd(), '.tmp/go-cache'),
  UV_CACHE_DIR:join(process.cwd(), '.tmp/uv-cache'), PYTHONUTF8:'1'};

function check(command, args) {
  const result = spawnSync(command, args, {env, encoding:'utf8', timeout:60000});
  return {command, args, status:result.status, stdout:result.stdout ?? '',
    stderr:result.stderr ?? '', error:result.error?.message};
}

const tracked = check('git', ['diff', '--name-only']);
const untracked = check('git', ['ls-files', '--others', '--exclude-standard']);
if (tracked.status !== 0 || untracked.status !== 0) throw Error('Changed-file inventory failed');
const goFiles = [...new Set(`${tracked.stdout}\n${untracked.stdout}`.split(/\r?\n/))]
  .filter(path => path.startsWith('internal/') && path.endsWith('.go'));
const scripts = readdirSync(root).filter(path => path.endsWith('.mjs')).map(path => join(root, path));
const python = readdirSync(root).filter(path => path.endsWith('.py')).map(path => join(root, path));
const results = [check('go', ['build', './...']), check('go', ['vet', '-tags', 'phase29preflight', './...']),
  check('gofmt', ['-l', ...goFiles]), check('git', ['diff', '--check']),
  check('.tmp/further-correction-20261004/check-functions.exe', goFiles),
  ...scripts.map(path => check('node', ['--check', path])),
  check('uv', ['run', 'python', '-c',
    'import ast,pathlib,sys; [ast.parse(pathlib.Path(p).read_text(encoding="utf-8"),filename=p) for p in sys.argv[1:]]', ...python])];
const passed = results.every(result => result.status === 0)
  && results.find(result => result.command === 'gofmt').stdout.trim() === '';
writeFileSync(`${root}/final-checks.json`, JSON.stringify({passed, results}, null, 2) + '\n');
console.log(JSON.stringify({passed, checks:results.length,
  failures:results.filter(result => result.status !== 0), go_files:goFiles.length}));
process.exitCode = passed ? 0 : 1;
