// Emits the client as JavaScript and loads it the way Node consumers do.
// Type-checking alone cannot catch a broken import: under moduleResolution
// "bundler", tsc resolves extensionless relative imports, but Node ESM
// requires the .js extension and throws ERR_MODULE_NOT_FOUND without it.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readdirSync, rmSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { create, equals, fromBinary, toBinary } from '@bufbuild/protobuf';

const root = fileURLToPath(new URL('.', import.meta.url));
const dist = join(root, 'dist');
rmSync(dist, { recursive: true, force: true });
const tsc = createRequire(import.meta.url).resolve('typescript/bin/tsc');
execFileSync(process.execPath, [tsc, '--noEmit', 'false', '--outDir', dist], { cwd: root, stdio: 'inherit' });

test('every emitted module loads in Node ESM', async () => {
  const modules = readdirSync(dist, { recursive: true }).filter((file) => file.endsWith('.js'));
  assert.ok(modules.includes('index.js'), 'tsc emitted no index.js');
  for (const file of modules) {
    await import(pathToFileURL(join(dist, file)).href);
  }
});

test('the entry point encodes and decodes a snapshot', async () => {
  const { SnapshotSchema } = await import(pathToFileURL(join(dist, 'index.js')).href);
  const snapshot = create(SnapshotSchema, {
    tickHeight: 9007199254740993n, // 2^53 + 1, beyond float64 precision
    version: 2,
    worldState: { nextId: 7, components: ['Position'], entities: [{ id: 6, components: [0], payloads: [Uint8Array.of(0x81, 0xa1, 0x58, 0x2a)] }] },
  });
  const decoded = fromBinary(SnapshotSchema, toBinary(SnapshotSchema, snapshot));
  assert.ok(equals(SnapshotSchema, snapshot, decoded));
  assert.equal(decoded.tickHeight, 9007199254740993n);
});
