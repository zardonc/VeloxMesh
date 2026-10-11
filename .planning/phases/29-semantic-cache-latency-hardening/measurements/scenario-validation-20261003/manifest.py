from pathlib import Path
import hashlib
import json
import subprocess
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parent
WORKSPACE = Path.cwd()

def checksum(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def main():
    binaries = ['app-linux.test','runner-profile.exe','supplemental-linux.test','app-windows.test.exe',
                'facts-linux.test','warmed-linux.test','warmed-windows.test.exe','warmed-runner.exe',
                'dimension-linux.test','final-scenario-linux.test']
    base = WORKSPACE/'.tmp'/'phase29-diagnostic-20261003'
    summary = json.loads((ROOT/'summary.json').read_text())
    manifest = dict(generated_utc=datetime.now(timezone.utc).isoformat(),
        source_head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),
        branch=subprocess.check_output(['git','branch','--show-current'],text=True).strip(),
        production_source_changed=False, merged=False, production_cache_enabled=False,
        test_invocations=len(summary['runs']), passed_invocations=sum(r['passed'] for r in summary['runs']),
        failed_logs=[r['log'] for r in summary['runs'] if not r['passed']],
        evidence_notes=['Initial chain-checks/scenarios-c4 binary hashes unavailable; final critical checks rerun.',
                        'P95 policy is diagnostic, not approved production SLO; original 1.05 gate remains failed.',
                        'Latest tests add real negative dimension and Redis delay; no production logic modified.'],
        binary_sha256={name:checksum(base/name) for name in binaries},
        checks={'go_build':'passed','go_vet_all_phase29preflight':'passed','runner_vet':'passed',
                'new_source_limits':'passed','git_diff_check':'passed','credential_audit':'passed'},
        cleanup='All task-owned test containers, processes, SSH forwards, HTTP servers and proxies closed; user local model server retained.')
    manifest['binary_groups'] = dict(main_suite='app-linux.test', supplemental_and_reverse='supplemental-linux.test',
        direct_and_sans_facts='facts-linux.test', warm_vm='warmed-linux.test', warm_host='warmed-windows.test.exe',
        unprimed_host='app-windows.test.exe', initial_dimension_assertion='dimension-linux.test',
        final_boundaries_and_chain='final-scenario-linux.test')
    manifest['artifact_sha256'] = {path.relative_to(ROOT).as_posix():checksum(path) for path in sorted(ROOT.rglob('*'))
                                  if path.is_file() and path.name!='manifest.json'}
    (ROOT/'manifest.json').write_text(json.dumps(manifest,indent=2))
    print(json.dumps({key:manifest[key] for key in ['test_invocations','passed_invocations','failed_logs','checks']},indent=2))

if __name__=='__main__':
    main()
