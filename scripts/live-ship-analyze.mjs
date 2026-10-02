import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const directory = process.argv[2];
if (!directory) throw new Error('Pass the evidence directory.');
const quantiles = (samples) => {
  const values = samples.map(({ elapsed_ms }) => elapsed_ms).sort((a, b) => a - b);
  const at = (fraction) => values[Math.ceil(values.length * fraction) - 1] ?? null;
  return { n: values.length, p50_ms: at(0.5), p95_ms: at(0.95), p99_ms: at(0.99) };
};

const analyze = (file) => {
  const log = readFileSync(join(directory, file), 'utf8');
  const samples = log.split('\n').filter((line) => line.includes('SAMPLE '))
    .map((line) => JSON.parse(line.slice(line.indexOf('SAMPLE ') + 'SAMPLE '.length)));
  const metadata = samples.find(({ type }) => type === 'metadata');
  const clients = samples.filter(({ type }) => type === 'client');
  const passed = /^--- PASS:/m.test(log) && !/^--- FAIL:/m.test(log);
  if (!metadata) return { file, passed, warmup: samples.find(({ type }) => type === 'warmup'), excluded: 'No completed load; inspect the run log.' };
  writeFileSync(join(directory, file.replace(/\.log$/, '.jsonl')), samples.map((row) => JSON.stringify(row)).join('\n') + '\n');
  const successful = clients.filter(({ ok }) => ok);
  const errors = Object.fromEntries([...new Set(clients.filter(({ ok }) => !ok).map(({ error }) => error))]
    .map((error) => [error, clients.filter((sample) => !sample.ok && sample.error === error).length]));
  return { file, passed, metadata, attempted: clients.length, ok: successful.length,
    failed: clients.length - successful.length, errors, hits: clients.filter(({ hit }) => hit).length,
    actual_rps: clients.length / (metadata.elapsed_ms / 1000), successful_rps: successful.length / (metadata.elapsed_ms / 1000),
    observed_concurrency: Math.max(...clients.map(({ concurrent }) => concurrent)),
    average_concurrency: clients.reduce((sum, { elapsed_ms }) => sum + elapsed_ms, 0) / metadata.elapsed_ms,
    schedule_lag_p95_ms: [...clients.map(({ start_ms }, index) => start_ms - index * metadata.interval_ms)]
      .sort((a, b) => a - b)[Math.ceil(clients.length * 0.95) - 1],
    full_response: quantiles(successful), all_attempts: quantiles(clients) };
};

const runs = readdirSync(directory).filter((file) => /^TestLiveCacheLoad.*\.log$/.test(file)).sort().map(analyze);
const pair = (offName, onName) => {
  const off = runs.find(({ file }) => file === offName);
  const on = runs.find(({ file }) => file === onName);
  const valid = off?.passed && on?.passed && off.ok === off.attempted && on.ok === on.attempted;
  const ratio = off?.full_response?.p95_ms && on?.full_response?.p95_ms
    ? on.full_response.p95_ms / off.full_response.p95_ms : null;
  return { off: offName, on: onName, complete_without_errors: Boolean(valid),
    p95_ratio: ratio, limit: 1.05, gate_passed: Boolean(valid && ratio <= 1.05) };
};
const summary = { runs, pairs: [pair('TestLiveCacheLoadOff.log', 'TestLiveCacheLoadOn.log'),
  pair('TestLiveCacheLoadOffRepeat.log', 'TestLiveCacheLoadOnRepeat.log')] };
writeFileSync(join(directory, 'load-summary.json'), JSON.stringify(summary, null, 2) + '\n');
process.stdout.write(JSON.stringify(summary, null, 2) + '\n');
