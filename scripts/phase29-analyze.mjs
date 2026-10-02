import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const directory = process.argv[2];
if (!directory) throw new Error('Pass the raw JSONL directory.');
const quantiles = (values) => {
  const sorted = [...values].sort((left, right) => left - right);
  const at = (fraction) => sorted[Math.ceil(sorted.length * fraction) - 1] ?? null;
  return { n: sorted.length, p50_ms: at(0.5), p95_ms: at(0.95), p99_ms: at(0.99) };
};

const analyze = (file) => {
  const raw = readFileSync(join(directory, file), 'utf8').trim();
  if (!raw) return { file, excluded: 'empty output; inspect retained run log for the failure' };
  const rows = raw.split('\n').map((line) => JSON.parse(line));
  const metadata = rows.find((row) => row.type === 'metadata');
  const clients = rows.filter((row) => row.type === 'client');
  const successful = clients.filter((row) => row.ok);
  const names = [...new Set(rows.filter((row) => row.type === 'operation').map((row) => row.name))];
  const operations = Object.fromEntries(names.map((name) => {
    const samples = rows.filter((row) => row.name === name);
    return [name, { ...quantiles(samples.map((row) => row.elapsed_ms)), errors: samples.filter((row) => !row.ok).length,
      successful: quantiles(samples.filter((row) => row.ok).map((row) => row.elapsed_ms)) }];
  }));
  return { file, metadata, attempted: clients.length, received_ok: successful.length,
    failed: clients.length - successful.length, hit_count: clients.filter((row) => row.hit).length,
    actual_rps: clients.length / (metadata.elapsed_ms / 1000), successful_rps: successful.length / (metadata.elapsed_ms / 1000),
    observed_concurrency: Math.max(...clients.map((row) => row.concurrent)),
    average_concurrency: clients.reduce((sum, row) => sum + row.elapsed_ms, 0) / metadata.elapsed_ms,
    full_response: quantiles(successful.map((row) => row.elapsed_ms)),
    attempts_including_failed: quantiles(clients.map((row) => row.elapsed_ms)), operations };
};

process.stdout.write(JSON.stringify(readdirSync(directory).filter((file) => file.endsWith('.jsonl')).sort().map(analyze), null, 2) + '\n');
