import {mkdirSync, readdirSync, readFileSync, writeFileSync} from 'node:fs';

const original = '.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner';
const destination = '.tmp/continued-investigation-20261004/runner';
const root = '.planning/phases/29-semantic-cache-latency-hardening/measurements/continued-investigation-20261004';
const prefix = 'veloxmesh-investigation-20261004-01a1089d';
mkdirSync(destination, {recursive:true});
for (const file of readdirSync(original).filter(name => name.endsWith('.go'))) {
  const source = readFileSync(`${original}/${file}`, 'utf8')
    .replaceAll('veloxmesh-test-', `${prefix}-`)
    .replaceAll('/tmp/veloxmesh-phase29-acceptance-20261001.test', `/tmp/${prefix}.test`)
    .replaceAll('.planning/phases/29-semantic-cache-latency-hardening/measurements/memory-upgrade-controlled-20261004/suite.mjs', `${root}/hardware-suite.mjs`)
    .replace('time.Sleep(controlledSettlePeriod)', 'if err := r.recoveryGate(); err != nil { return err }')
    .replace('cat /proc/stat /proc/loadavg /proc/uptime', 'cat /proc/stat /proc/loadavg /proc/uptime /proc/meminfo /proc/pressure/cpu /proc/pressure/io /proc/pressure/memory /proc/diskstats')
    .replace('Dependencies ready; 15-second recovery period outside formal test windows','Dependencies ready; existing 10-second quiet gate required before hardware screen')
    .replace('if len(os.Args) > 1 && os.Args[1] == "controlled" {', 'if len(os.Args) > 1 && os.Args[1] == "backend" {\n if err := r.dependencies(); err != nil { return err }; if err := r.forward(); err != nil { return err }; return r.localChecks()\n }\n if len(os.Args) > 1 && os.Args[1] == "controlled" {');
  writeFileSync(`${destination}/${file}`, source);
}
const suite = readFileSync('.planning/phases/29-semantic-cache-latency-hardening/measurements/memory-upgrade-controlled-20261004/suite.mjs','utf8');
writeFileSync(`${root}/controlled-suite.mjs`, suite);
