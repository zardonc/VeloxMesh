import {Agent, request} from 'node:http';
import {appendFileSync, mkdirSync, readFileSync, writeFileSync} from 'node:fs';
import {createHash} from 'node:crypto';

const [base, model, root] = process.argv.slice(2);
const token = process.env.PHASE29_UPSTREAM_API_KEY;
const endpoint = new URL(`${base}/chat/completions`);
if (endpoint.hostname !== '127.0.0.1' || endpoint.protocol !== 'http:' || !model || !root || !token) throw Error('Explicit local test inputs required');
const windowCount = 32, responseLimit = 65536, requestTimeout = 4000, hardTimeout = 55000, cancelCount = 8, cancelAfter = 10;
const fixture = readFileSync('internal/app/phase29_live_test.go', 'utf8').match(/const liveFAQSystem = ("[^\n]+")/);
if (!fixture) throw Error('Original FAQ fixture unavailable');
const system = JSON.parse(fixture[1]);
mkdirSync(root, {recursive: true});
const agents = [];
const samples = [];
const summaries = [];
let sequence = 0;
const hardTimer = setTimeout(() => {
  appendFileSync(`${root}/upstream-replay.jsonl`, JSON.stringify({type: 'hard_timeout', time: new Date().toISOString()}) + '\n');
  for (const agent of agents) agent.destroy();
  process.exit(124);
}, hardTimeout);

function call(options) {
  return new Promise(resolve => {
    const started = performance.now(), id = ++sequence;
    const payload = JSON.stringify({model, messages: [{role: 'system', content: system}, {role: 'user', content: `How long is the trial for plan ${id % 2 ? 70 : 64}?`}], temperature: 0, max_tokens: 256});
    let finished = false;
    const finish = result => {
      if (finished) return;
      finished = true;
      const sample = {type: 'upstream_request', time: new Date().toISOString(), id, window: options.label, elapsed_ms: performance.now() - started, reused_socket: req.reusedSocket, ...result};
      samples.push(sample);
      appendFileSync(`${root}/upstream-replay.jsonl`, JSON.stringify(sample) + '\n');
      resolve(sample);
    };
    const req = request(endpoint, {method: 'POST', agent: options.agent, headers: {Authorization: `Bearer ${token}`, 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(payload), 'X-Request-ID': `engine-replay-${id}`}}, response => {
      const chunks = [];
      let size = 0;
      response.on('data', chunk => {
        size += chunk.length;
        if (size > responseLimit) response.destroy(Error('response_limit'));
        else chunks.push(chunk);
      });
      response.on('error', error => finish({status: response.statusCode, ok: false, error: error.message}));
      response.on('end', () => {
        const body = Buffer.concat(chunks).toString('utf8');
        let parsed;
        try { parsed = JSON.parse(body); } catch { parsed = null; }
        const ok = response.statusCode === 200 && !!parsed?.choices?.[0]?.message?.content;
        finish({status: response.statusCode, ok, tokens: parsed?.usage?.total_tokens,
          body_sha256: createHash('sha256').update(body).digest('hex'),
          error_body: ok ? undefined : body.replaceAll(token, '[REDACTED]')});
      });
    });
    req.on('error', error => finish({ok: false, error: error.message, code: error.code, cause: error.cause?.code}));
    req.setTimeout(requestTimeout, () => req.destroy(Error('request_timeout')));
    if (options.cancel) setTimeout(() => req.destroy(Error('intentional_client_cancel')), cancelAfter);
    req.end(payload);
  });
}

async function window(options) {
  const agent = new Agent({keepAlive: options.keepAlive, maxSockets: options.concurrency});
  agents.push(agent);
  const started = performance.now(), first = samples.length;
  let next = 0;
  await Promise.all(Array.from({length: options.concurrency}, async () => {
    while (next++ < windowCount) await call({...options, agent});
  }));
  const rows = samples.slice(first), sorted = rows.map(row => row.elapsed_ms).sort((a, b) => a - b);
  summaries.push({...options, requests: rows.length, failed: rows.filter(row => !row.ok).length,
    reused_requests: rows.filter(row => row.reused_socket).length, elapsed_ms: performance.now() - started,
    p95_ms: sorted[Math.ceil(sorted.length * 0.95) - 1]});
  agent.destroy();
}

try {
  for (const options of [
    {label: 'c1-reuse', concurrency: 1, keepAlive: true},
    {label: 'c4-fresh', concurrency: 4, keepAlive: false},
    {label: 'c4-reuse', concurrency: 4, keepAlive: true},
    {label: 'c1-fresh', concurrency: 1, keepAlive: false},
  ]) await window(options);
  const cancelAgent = new Agent({keepAlive: true, maxSockets: 1});
  agents.push(cancelAgent);
  for (let index = 0; index < cancelCount; index++) await call({label: 'intentional-cancel', agent: cancelAgent, cancel: true});
  cancelAgent.destroy();
  await window({label: 'c4-after-cancel', concurrency: 4, keepAlive: true});
  writeFileSync(`${root}/upstream-replay-summary.json`, JSON.stringify({model, endpoint: endpoint.href, summaries,
    cancellations: samples.filter(row => row.window === 'intentional-cancel')}, null, 2) + '\n');
  console.log(JSON.stringify({summaries, failures: samples.filter(row => !row.ok && row.window !== 'intentional-cancel')}, null, 2));
  process.exitCode = summaries.some(row => row.failed > 0) ? 1 : 0;
} finally {
  clearTimeout(hardTimer);
  for (const agent of agents) agent.destroy();
}
