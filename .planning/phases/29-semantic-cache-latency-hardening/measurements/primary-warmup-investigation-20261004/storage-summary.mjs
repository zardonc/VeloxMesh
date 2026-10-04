import {readFileSync, writeFileSync} from 'node:fs';
import {basename, dirname} from 'node:path';
import {fileURLToPath} from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const rows = readFileSync(`${root}/storage-file-layout.log`, 'utf8').trim().split(/\r?\n/).map(line => {
  const [bytes, blocks, file] = line.split('\t');
  return {bytes: Number(bytes), allocated_bytes: Number(blocks)*512, file};
});
const groups = Object.groupBy(rows, row => row.file.includes('/wal/') ? 'wal'
  : basename(row.file).replace(/[a-f0-9]{32,}/g, '<hash>').replace(/\d+/g, '#'));
const result = {files: rows.length, groups: Object.entries(groups).map(([family, items]) => ({family,
  count: items.length, bytes: items.reduce((sum, row) => sum+row.bytes, 0),
  allocated_bytes: items.reduce((sum, row) => sum+row.allocated_bytes, 0), example: items[0].file}))
  .sort((a, b) => b.bytes-a.bytes)};
writeFileSync(`${root}/storage-layout-summary.json`, JSON.stringify(result, null, 2)+'\n');
console.log(JSON.stringify({files: result.files, families: result.groups.length}));
