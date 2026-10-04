import json
import subprocess

COMMANDS = [
    ['date', '-u'], ['uptime'], ['free', '-m'],
    ['docker', 'ps', '-a', '--format', '{{.Names}} {{.State}} {{.Image}}'],
    ['docker', 'inspect', '--format', '{{.Name}} {{.Image}} {{json .Mounts}} {{json .HostConfig.PortBindings}}',
     'veloxmesh-test-redis', 'veloxmesh-test-qdrant', 'veloxmesh-test-postgres'],
    ['ps', '-eo', 'pid,etimes,comm'], ['ss', '-ltn'],
]
for command in COMMANDS:
    result = subprocess.run(command, capture_output=True, text=True, timeout=8, check=False)
    print(json.dumps({'command': command, 'status': result.returncode, 'stdout': result.stdout, 'stderr': result.stderr}))
    if result.returncode:
        raise SystemExit(result.returncode)
