import {mkdirSync, writeFileSync} from 'node:fs';
import {performance} from 'node:perf_hooks';

const [base, embeddingModel, chatModel, output, countText = '100', concurrencyText = '4'] = process.argv.slice(2);
const count = Number(countText), concurrency = Number(concurrencyText);
if (!base || !embeddingModel || !chatModel || !output || count < 1 || count > 1000 || concurrency < 1) throw Error('explicit bounded inputs required');
const system = "Test static FAQ: each numbered plan has a trial lasting its plan number in days. Answer the requested plan's trial length in one short sentence.";

async function request(options) {
  const started = performance.now();
  try {
    const response = await fetch(`${base}/${options.path}`, {method:'POST', headers:{'Content-Type':'application/json'},
      body:JSON.stringify(options.body), signal:AbortSignal.timeout(options.timeout)});
    const body = await response.json();
    if (!response.ok) throw Error(`HTTP_${response.status}: ${body.error?.message ?? 'unknown'}`);
    return {elapsed_ms:performance.now()-started, ok:true, body};
  } catch (error) { return {elapsed_ms:performance.now()-started, ok:false, error:String(error)}; }
}

async function embedding(index) {
  const result = await request({path:'embeddings', body:{model:embeddingModel, input:[`How long is the trial for plan ${index+1}?`]}, timeout:2000});
  const vector = result.body?.data?.[0]?.embedding;
  const {body, ...summary} = result;
  const valid = vector && vector.every(Number.isFinite) && vector.some(x=>x!==0);
  return {...summary, dimension:vector?.length ?? 0, ok:result.ok && Boolean(valid), index};
}

async function chat(index) {
  const result = await request({path:'chat/completions', body:{model:chatModel, temperature:0, max_tokens:256,
    messages:[{role:'system',content:system},{role:'user',content:`How long is the trial for plan ${index+1}?`}]}, timeout:12000});
  const {body, ...summary} = result;
  return {...summary, ok:result.ok && Boolean(body?.choices?.[0]?.message?.content), index};
}

async function window(coLoad, dimension) {
  let next = 0;
  const started = performance.now();
  const workers = await Promise.all(Array.from({length:concurrency}, async()=>{
    let records = [], chats = [];
    while (next < count) {
      const index = next++;
      const work = coLoad ? chat(index) : null;
      const sample = await embedding(index);
      records = [...records, {...sample, ok:sample.ok && sample.dimension === dimension}];
      if (work) chats = [...chats, await work];
    }
    return {records, chats};
  }));
  return {co_load:coLoad, concurrency, elapsed_ms:performance.now()-started,
    records:workers.flatMap(w=>w.records), chats:workers.flatMap(w=>w.chats)};
}

const probe = await embedding(999);
if (!probe.ok) throw Error(`real dimension probe failed: ${JSON.stringify(probe)}`);
const standalone = await window(false, probe.dimension);
const withChat = await window(true, probe.dimension);
const windows = [standalone, withChat];
mkdirSync(output, {recursive:true});
writeFileSync(`${output}/local-embedding.json`, JSON.stringify({base, embedding_model:embeddingModel, chat_model:chatModel,
  dimension:probe.dimension, probe, windows}, null, 2));
console.log(JSON.stringify({output, failures:windows.map(w=>w.records.filter(x=>!x.ok).length), dimension:probe.dimension}));
