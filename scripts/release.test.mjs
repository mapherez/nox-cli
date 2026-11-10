import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { parseVersion, release } from './release.mjs';

// Git is simulated: these tests cannot create a real commit, tag or remote push.
function fixture(t, options = {}) {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'nox-cli-release-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const path = join(root, 'package.json');
  const original = '{\r\n  "name": "nox-cli",\r\n  "version": "0.0.0",\r\n  "private": true\r\n}\r\n';
  writeFileSync(path, original);
  writeFileSync(join(root, 'go.mod'), 'module github.com/mapherez/nox-cli\n');
  let head = 'before', staged = false;
  const calls = [];
  const run = (command, args) => {
    assert.equal(command, 'git');
    calls.push(args);
    const key = args.join(' ');
    if (key === 'rev-parse --show-toplevel') return root;
    if (args[0] === 'status') return options.dirty ? ' M README.md\0' : head === 'before' && readFileSync(path, 'utf8') !== original ? ' M package.json\0' : '';
    if (args[0] === 'symbolic-ref') return 'main';
    if (args.includes('var')) return 'Test Author <author@example.invalid>';
    if (key === 'rev-parse --verify HEAD^{commit}' || key === 'rev-parse HEAD') return head;
    if (args[0] === 'ls-files') return 'package.json';
    if (args[0] === 'tag' && args[1] === '--list') return options.existingTag ? 'v0.1.0' : '';
    if (args[0] === 'ls-remote') {
      if (options.remoteFailure) throw new Error('Remote unavailable');
      return '';
    }
    if (args[0] === 'add') { staged = true; return ''; }
    if (args[0] === 'diff') return staged ? 'package.json' : '';
    if (args[0] === 'commit') {
      if (options.commitFailure) throw new Error('Commit failed');
      head = 'release-commit'; staged = false; return '';
    }
    if (args[0] === 'restore') { staged = false; return ''; }
    if (args[0] === 'diff-tree') return 'package.json';
    if (args[0] === 'show') return readFileSync(path, 'utf8');
    if (args[0] === 'tag') return '';
    if (args[0] === 'rev-parse' && args[1] === 'v0.1.0^{commit}') return head;
    if (args[0] === 'push') {
      if (options.pushFailure) throw new Error('Push failed');
      return '';
    }
    throw new Error(`Unexpected Git command: ${key}`);
  };
  return { root, path, original, calls, run };
}

test('only stable versions supported by the existing package workflow are accepted', () => {
  assert.deepEqual(parseVersion('v0.1.0'), { version: '0.1.0', tag: 'v0.1.0' });
  for (const input of ['', '01.0.0', '1.0.0\n', ' 1.0.0', '1.0.0-rc.1', '1.0.0+build', '1.0']) {
    assert.throws(() => parseVersion(input));
  }
});

test('prepares only the manifest, tags the commit and requests an atomic push', t => {
  const f = fixture(t);
  const result = release(['0.1.0'], { ...f, cwd: f.root, log() {} });
  assert.equal(result.tag, 'v0.1.0');
  assert.equal(JSON.parse(readFileSync(f.path)).version, '0.1.0');
  assert.match(readFileSync(f.path, 'utf8'), /\r\n/);
  assert.deepEqual(f.calls.at(-1), ['push', '--atomic', 'origin', 'refs/heads/main:refs/heads/main', 'refs/tags/v0.1.0:refs/tags/v0.1.0']);
});

for (const condition of ['dirty', 'existingTag', 'remoteFailure', 'commitFailure']) {
  test(`${condition} aborts without leaving a version change or pushing`, t => {
    const f = fixture(t, { [condition]: true });
    assert.throws(() => release(['0.1.0'], { ...f, cwd: f.root, log() {} }));
    assert.equal(readFileSync(f.path, 'utf8'), f.original);
    assert.equal(f.calls.some(args => args[0] === 'push'), false);
  });
}

test('a push failure preserves the release commit/version and provides recovery', t => {
  const f = fixture(t, { pushFailure: true });
  assert.throws(() => release(['0.1.0'], { ...f, cwd: f.root, log() {} }), /remain local.*No automatic reset/s);
  assert.equal(JSON.parse(readFileSync(f.path)).version, '0.1.0');
  assert.equal(f.calls.some(args => args[0] === 'reset'), false);
});
