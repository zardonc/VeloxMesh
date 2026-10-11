import hashlib
import json
import os
import stat
import subprocess
import time
import urllib.request

PREFIX = 'veloxmesh-investigation-20261004-01a1089d'
KINDS = ('redis', 'qdrant', 'postgres')

def command(args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=10)
    if result.returncode:
        raise RuntimeError(f'{args[:3]}: {result.stderr}')
    return result.stdout.strip()

owned = json.loads(command(['docker', 'inspect', *(PREFIX+'-'+kind for kind in KINDS)]))
for item in owned:
    if item['Config']['Labels'].get('veloxmesh.investigation') != '01a1089d':
        raise RuntimeError('Ownership mismatch; cleanup refused')
    if item['State']['Running']:
        raise RuntimeError('Test still running; cleanup refused')

qdrant = next(item for item in owned if item['Name'].endswith('-qdrant'))
private_env = dict(value.split('=',1) for value in qdrant['Config']['Env'] if '=' in value)
command(['docker', 'start', PREFIX+'-qdrant'])
try:
    headers = {'api-key': private_env['QDRANT__SERVICE__API_KEY']}
    endpoint = 'http://127.0.0.1:6333/collections'
    for attempt in range(10):
        try:
            with urllib.request.urlopen(urllib.request.Request(endpoint, headers=headers), timeout=2) as response:
                collections = json.load(response)['result']['collections']
            break
        except OSError:
            if attempt == 9:
                raise
            time.sleep(1)
    print(json.dumps({'fresh_collection_count': len(collections),
                      'semantic_count': sum(row['name'].startswith('semantic_cache_') for row in collections),
                      'names': [row['name'] for row in collections]}), flush=True)
finally:
    command(['docker', 'stop', '--time', '3', PREFIX+'-qdrant'])

path = '/tmp/' + PREFIX + '.test'
if os.path.exists(path):
    metadata = os.lstat(path)
    if not stat.S_ISREG(metadata.st_mode) or metadata.st_uid != os.getuid():
        raise RuntimeError('Owned temporary binary identity mismatch')
    with open(path, 'rb') as binary:
        digest = hashlib.file_digest(binary, 'sha256').hexdigest()
    os.unlink(path)
    print(json.dumps({'removed_owned_binary': path, 'sha256': digest, 'size': metadata.st_size}), flush=True)

for item in json.loads(command(['docker', 'inspect', *(PREFIX+'-'+kind for kind in KINDS),
                               *('veloxmesh-test-'+kind for kind in KINDS)])):
    print(json.dumps({'name': item['Name'], 'running': item['State']['Running'],
                      'oom': item['State']['OOMKilled'], 'restarts': item['RestartCount'],
                      'volume_preserved': [mount.get('Name') for mount in item['Mounts']]}), flush=True)
print(command(['ss', '-ltn']))
