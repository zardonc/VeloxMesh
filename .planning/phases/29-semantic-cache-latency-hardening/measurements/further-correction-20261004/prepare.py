import json
import socket
import subprocess

PREFIX = 'veloxmesh-further-20261004-01a1089d'
LABEL = 'veloxmesh.investigation=01a1089d-further'
DEPENDENCIES = ('redis', 'qdrant', 'postgres')

def command(args, **options):
    result = subprocess.run(args, capture_output=True, text=True, timeout=12, **options)
    if result.returncode:
        raise RuntimeError(f'command {args[:3]} failed: {result.stderr}')
    return result.stdout.strip()

active = command(['docker', 'ps', '-q'])
if active:
    raise RuntimeError('Active container collision; no environment changes made')
existing = command(['docker', 'ps', '-a', '--format', '{{.Names}}']).splitlines()
volumes = command(['docker', 'volume', 'ls', '--format', '{{.Name}}']).splitlines()
if any(name.startswith(PREFIX) for name in existing + volumes):
    raise RuntimeError('Fixture already exists; refusing reuse')
for port in (6379, 6333, 6334, 5432, 11234):
    with socket.socket() as connection:
        connection.bind(('127.0.0.1', port))

created = []
try:
    for kind in DEPENDENCIES:
        original = json.loads(command(['docker', 'inspect', 'veloxmesh-test-' + kind]))[0]
        name = PREFIX + '-' + kind
        command(['docker', 'volume', 'create', '--label', LABEL, name])
        args = ['docker', 'create', '--pull', 'never', '--name', name, '--label', LABEL,
                '--env-file', '/dev/stdin']
        for port in original['HostConfig']['PortBindings']:
            host_port = original['HostConfig']['PortBindings'][port][0]['HostPort']
            args += ['-p', '127.0.0.1:' + host_port + ':' + port]
        for mount in original['Mounts']:
            args += ['--mount', 'type=volume,source=' + name + ',target=' + mount['Destination']]
        args += [original['Image']] + (original['Config']['Cmd'] or [])
        identifier = command(args, input='\n'.join(original['Config']['Env']) + '\n')
        created.append(name)
        print(json.dumps({'created': name, 'id': identifier, 'image': original['Image'],
                          'volume': name, 'private_env_inherited': True}), flush=True)
except Exception:
    for name in created:
        print(json.dumps({'stopped_after_prepare_failure': name,
                          'result': command(['docker', 'stop', '--time', '2', name])}), flush=True)
    raise
