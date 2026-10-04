import {existsSync,readFileSync,mkdirSync,writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
// A task-local pause finishes the active runner first, preserving its cleanup.
if (process.env.SHIP_RESULTS_ROOT && existsSync(`${process.env.SHIP_RESULTS_ROOT}/PAUSE`)) process.exit(2);
const cfg=Object.fromEntries(readFileSync('.env.local','utf8').split(/\r?\n/).filter(x=>/^\w+=/.test(x)).map(x=>{const i=x.indexOf('=');return [x.slice(0,i),x.slice(i+1).replace(/^['"]|['"]$/g,'')];}));
const [provider,model,tests='TestLiveModelAvailability',label='preflight',count='16',interval='1000',concurrency='2']=process.argv.slice(2);
const out=`${process.env.SHIP_RESULTS_ROOT??'.planning/phases/29-semantic-cache-latency-hardening/measurements/diagnostic-20261003'}/${label}/${provider}-${model.replace(/[^\w.-]/g,'_')}`;
mkdirSync(out,{recursive:true});
const env={...process.env,SHIP_ARTIFACTS:out,SHIP_BINARY:process.env.SHIP_BINARY??'.tmp/phase29-diagnostic-20261003/app-linux.test',SHIP_PROVIDER:provider,SHIP_MODEL:model,SHIP_TESTS:tests,PHASE29_MODEL:'text-embedding-embedder_collection',PHASE29_EMBEDDING_BASE_URL:'http://127.0.0.1:11234/v1',PHASE29_EMBEDDING_API_KEY:'local-test',PHASE29_READ_TIMEOUT:'100ms',PHASE29_READ_CONCURRENCY:'4',PHASE29_WRITE_WORKERS:'2',PHASE29_QUEUE_CAPACITY:'32',PHASE29_WRITE_TIMEOUT:'2s',PHASE29_SHUTDOWN_GRACE:'1s',PHASE29_COUNT:count,PHASE29_INTERVAL_MS:interval,PHASE29_CLIENT_CONCURRENCY:concurrency};
for(const key of Object.keys(env)){if(key.startsWith('PHASE29_') && process.env[key]!==undefined)env[key]=process.env[key];}
if(provider==='LOCAL'){Object.assign(env,{SHIP_LOCAL_BASE_URL:'http://127.0.0.1:11234/v1',SHIP_LOCAL_MODEL:model,SHIP_LOCAL_API_KEY:'local-test'});}
if(process.env.PHASE29_MIXED==='true'){
 const entries=[['OR','inclusionai/ling-3.0-flash-sante:free'],['SANS','oc/space-bunny-free'],['SANS','oc/longcat-2.5-preview-free'],['LOCAL','qwen2.5-0.5b-instruct']];
 env.PHASE29_EXTRA_PROVIDERS=JSON.stringify(entries.map(([p,m],i)=>({id:'diagnostic-'+i,type:'openai-compatible',base_url:p==='LOCAL'?'http://127.0.0.1:11234/v1':cfg[p+'_BASE_URL'],auth:{api_key_env:p==='LOCAL'?'PHASE29_EMBEDDING_API_KEY':p+'_PRIMARY_API_KEY'},models:[m]})));
 env.PHASE29_MIXED_MODELS=[model,...entries.map(x=>x[1])].join(',');env.OR_PRIMARY_API_KEY=cfg.OR_PRIMARY_API_KEY;
}
const run=spawnSync(process.env.SHIP_RUNNER??'.tmp/phase29-diagnostic-20261003/runner.exe',[],{env,encoding:'utf8',timeout:tests.split(',').length*80000+30000});
writeFileSync(`${out}/runner.log`,`${run.stdout??''}${run.stderr??''}`);
console.log(JSON.stringify({provider,model,label,status:run.status,error:run.error?.message,output:run.stdout?.trim(),stderr:run.stderr?.trim()}));
process.exitCode=run.status??1;
