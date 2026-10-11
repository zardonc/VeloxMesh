import {writeFileSync} from 'node:fs';
import {performance} from 'node:perf_hooks';

const [base, embeddingModel, chatModel, output] = process.argv.slice(2);
if (!base || !embeddingModel || !chatModel || !output) throw Error('explicit API/models/output required');
const count = 100, concurrency = 4, interval = 125, timeout = 12000;
const system = "Test static FAQ: each numbered plan has a trial lasting its plan number in days. Answer the requested plan's trial length in one short sentence.";
async function request(kind, index, origin, planned) {
  const started = performance.now();
  const body = kind === 'embedding' ? {model:embeddingModel, input:[`How long is the trial for plan ${index+1}?`]} :
    {model:chatModel, temperature:0, max_tokens:256, messages:[{role:'system', content:system}, {role:'user', content:`How long is the trial for plan ${index+1}?`}]};
  try {
    const response = await fetch(`${base}/${kind === 'embedding' ? 'embeddings' : 'chat/completions'}`,
      {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body), signal:AbortSignal.timeout(timeout)});
    const headers = performance.now();
    const data = await response.json();
    const vector = data.data?.[0]?.embedding;
    const valid = kind === 'embedding' ? vector?.length === 768 && vector.every(Number.isFinite) && vector.some(v=>v!==0) : Boolean(data.choices?.[0]?.message?.content);
    return {kind, index, status:response.status, ok:response.ok && Boolean(valid), dimension:vector?.length,
      start_ms:started-origin, planned_ms:planned, headers_ms:headers-started, elapsed_ms:performance.now()-started};
  } catch(error) {
    return {kind, index, ok:false, error:String(error), start_ms:started-origin, planned_ms:planned, elapsed_ms:performance.now()-started};
  }
}

async function stream(kind, origin) {
  let records = [], pending = new Set();
  for (let index=0; index<count; index++) {
    const planned = index*interval, delay = origin+planned-performance.now();
    if (delay>0) await new Promise(resolve=>setTimeout(resolve,delay));
    if (pending.size >= concurrency) await Promise.race(pending);
    const work = request(kind,index,origin,planned).then(sample=>{records=[...records,sample];pending.delete(work);});
    pending.add(work);
  }
  await Promise.all(pending);
  return {kind, records};
}

const probe = await request('embedding',999,performance.now(),0);
if (!probe.ok) throw Error('actual embedding preflight failed');
const chatProbe = await request('chat',999,performance.now(),0);
if (!chatProbe.ok) throw Error('actual chat preflight failed');
const standaloneOrigin = performance.now();
const standalone = await stream('embedding',standaloneOrigin);
const togetherOrigin = performance.now();
const together = await Promise.all([stream('embedding',togetherOrigin),stream('chat',togetherOrigin)]);
const failures = [standalone,...together].reduce((n,s)=>n+s.records.filter(r=>!r.ok).length,0);
writeFileSync(output,JSON.stringify({at:new Date().toISOString(), base, embedding_model:embeddingModel, chat_model:chatModel,
  count, concurrency_per_stream:concurrency, interval_ms:interval, arrival_schedules:'independent; dispatch lag retained',
  probe, chat_probe:chatProbe, standalone, together, failures},null,2)+'\n');
console.log(JSON.stringify({output, failures}));
process.exitCode = failures ? 1 : 0;
