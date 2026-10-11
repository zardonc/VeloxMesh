import {readFileSync, mkdirSync, writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';

const [binary, output, concurrency = '4', order = 'observed-first'] = process.argv.slice(2);
if (!binary || !output || !process.env.DIRECT_EMBEDDING_BASE_URL || !process.env.PHASE29_MODEL) throw Error('explicit local Go inputs required');
const cfg = Object.fromEntries(readFileSync('.env.local','utf8').split(/\r?\n/).filter(x=>/^\w+=/.test(x)).map(x=>{
  const index = x.indexOf('=');
  return [x.slice(0,index),x.slice(index+1).replace(/^['"]|['"]$/g,'')];
}));
const input = {PHASE29_EMBEDDING_BASE_URL:process.env.DIRECT_EMBEDDING_BASE_URL,
  PHASE29_EMBEDDING_API_KEY:cfg.LOC_PRIMARY_API_KEY ?? '', PHASE29_COUNT:'100', PHASE29_CLIENT_CONCURRENCY:concurrency,
  PHASE29_EMBEDDING_PARALLEL_WARMUP:process.env.PHASE29_EMBEDDING_PARALLEL_WARMUP ?? '0',
  SANS_BASE_URL:process.env.DIRECT_EMBEDDING_BASE_URL,SANS_PRIMARY_API_KEY:cfg.LOC_PRIMARY_API_KEY ?? '',SANS_PRIMARY_DEFAULT_MODEL:process.env.SHIP_LOCAL_MODEL};
mkdirSync(output,{recursive:true});
const tests = ['TestLiveEmbeddingStress','TestLiveEmbeddingStressBare'];
for (const test of order === 'reverse' ? [...tests].reverse() : tests) {
  const child = spawnSync(binary,[`-test.run=^${test}$`,'-test.v','-test.timeout=60s'],{env:process.env,
    input:JSON.stringify(input),encoding:'utf8',timeout:60000});
  writeFileSync(`${output}/${test}.log`,`${child.stdout ?? ''}${child.stderr ?? ''}`);
  console.log(JSON.stringify({test,output,status:child.status,error:child.error?.message}));
  if (child.status !== 0) process.exitCode = 1;
}
