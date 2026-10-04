import {readFileSync, writeFileSync, mkdirSync} from 'node:fs';
import {spawnSync} from 'node:child_process';

const selected = JSON.parse(process.env.SHIP_SELECTED_MODELS ?? '[]');
if (selected.length < 2 || !process.env.SHIP_RUNNER || !process.env.SHIP_BINARY || !process.env.SHIP_RESULTS_ROOT) {
  throw Error('explicit verified models and replay paths required');
}
const cfg = Object.fromEntries(readFileSync('.env.local', 'utf8').split(/\r?\n/).filter(line => /^\w+=/.test(line)).map(line => {
  const position = line.indexOf('=');
  return [line.slice(0, position), line.slice(position + 1).replace(/^['"]|['"]$/g, '')];
}));
const [primary, ...extra] = selected;
if (extra.some(item => item.provider !== 'OR')) throw Error('this replay uses only the already verified extra OR provider');
const additional = extra.map((item, index) => ({id: `capacity-provider-${index}`, type: 'openai-compatible',
  base_url: cfg[`${item.provider}_BASE_URL`], auth: {api_key_env: `${item.provider}_PRIMARY_API_KEY`}, models: [item.model]}));
const helper = '.planning/phases/29-semantic-cache-latency-hardening/measurements/scenario-validation-20261003/run.mjs';
const env = {...process.env, PHASE29_EXTRA_PROVIDERS: JSON.stringify(additional),
  PHASE29_MIXED_MODELS: selected.map(item => item.model).join(','), OR_PRIMARY_API_KEY: cfg.OR_PRIMARY_API_KEY};
let failures = 0;
for (const test of ['TestLiveMixedLoadOff', 'TestLiveMixedLoadOn']) {
  const args = [helper, primary.provider, primary.model, test, 'mixed-final', '24', '250', '3'];
  const child = spawnSync(process.execPath, args, {env, encoding: 'utf8', timeout: 110000});
  const directory = `${process.env.SHIP_RESULTS_ROOT}/mixed-final`;
  mkdirSync(directory, {recursive: true});
  writeFileSync(`${directory}/${test}-dispatch.json`, JSON.stringify({models: selected, status: child.status,
    output: child.stdout, stderr: child.stderr, error: child.error?.message}, null, 2) + '\n');
  console.log(JSON.stringify({test, status: child.status}));
  if (child.status !== 0) failures++;
}
process.exitCode = failures ? 1 : 0;
