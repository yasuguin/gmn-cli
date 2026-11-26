'use strict';

// ランチャーの単体テスト。実行は `bun test packaging/npm/test/`（単体テストは bun:test）。
const { test } = require('bun:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { resolveBinary, unsupportedMessage, SUPPORTED } = require('../lib/platform');
const { run } = require('../lib/launcher');

test('対応する 6 種の OS・arch は OS 別パッケージの実行ファイルを返す', () => {
  const resolve = (id) => `/fake/node_modules/${id}`;
  const cases = [
    ['darwin', 'arm64', '@yasuguin/gmn-cli-darwin-arm64/gmn'],
    ['darwin', 'x64', '@yasuguin/gmn-cli-darwin-x64/gmn'],
    ['linux', 'arm64', '@yasuguin/gmn-cli-linux-arm64/gmn'],
    ['linux', 'x64', '@yasuguin/gmn-cli-linux-x64/gmn'],
    ['win32', 'arm64', '@yasuguin/gmn-cli-win32-arm64/gmn.exe'],
    ['win32', 'x64', '@yasuguin/gmn-cli-win32-x64/gmn.exe'],
  ];
  assert.equal(SUPPORTED.length, 6);
  for (const [platform, arch, id] of cases) {
    assert.deepEqual(resolveBinary(platform, arch, resolve), { path: `/fake/node_modules/${id}` });
  }
});

test('対応していない OS・arch は対応表を示すエラーになる', () => {
  const result = resolveBinary('freebsd', 'x64', () => {
    throw new Error('呼ばれないはず');
  });
  assert.ok(result.error, 'error が返るはず');
  assert.match(result.error, /freebsd-x64 は対応していない/);
  for (const e of SUPPORTED) {
    assert.ok(result.error.includes(`${e.platform}-${e.arch}`), `対応表に ${e.platform}-${e.arch} が無い`);
  }
  assert.equal(result.error, unsupportedMessage('freebsd', 'x64'));
});

test('対応する OS でも arch が対応外なら対応表のエラーになる', () => {
  const result = resolveBinary('linux', 'ia32', () => '/never');
  assert.match(result.error, /linux-ia32 は対応していない/);
});

test('対応する OS でもパッケージが入っていなければ入れ直すよう案内する', () => {
  const resolve = () => {
    const err = new Error('Cannot find module');
    err.code = 'MODULE_NOT_FOUND';
    throw err;
  };
  const result = resolveBinary('linux', 'x64', resolve);
  assert.match(result.error, /@yasuguin\/gmn-cli-linux-x64 が入っていない/);
});

test('run は子プロセスの終了コードをそのまま返す', () => {
  const calls = [];
  const exits = [];
  const child = new EventEmitter();
  const stderr = { write: (s) => calls.push(s) };
  run(['--version'], {
    platform: 'linux',
    arch: 'x64',
    resolve: (id) => `/fake/${id}`,
    spawn: (path, argv, opts) => {
      calls.push({ path, argv, stdio: opts.stdio });
      return child;
    },
    exit: (code) => exits.push(code),
    kill: () => calls.push('kill'),
    stderr,
  });
  assert.deepEqual(calls[0], {
    path: '/fake/@yasuguin/gmn-cli-linux-x64/gmn',
    argv: ['--version'],
    stdio: 'inherit',
  });
  child.emit('exit', 3, null);
  assert.deepEqual(exits, [3]);
});

test('run は子がシグナルで終わったら同じシグナルで親も終わる', () => {
  const kills = [];
  const exits = [];
  const child = new EventEmitter();
  run([], {
    platform: 'darwin',
    arch: 'arm64',
    resolve: (id) => `/fake/${id}`,
    spawn: () => child,
    exit: (code) => exits.push(code),
    kill: (sig) => kills.push(sig),
    stderr: { write() {} },
  });
  child.emit('exit', null, 'SIGTERM');
  assert.deepEqual(kills, ['SIGTERM']);
  assert.deepEqual(exits, []);
});

test('run は対応外の環境で子を起動せず終了コード 1 で止まる', () => {
  const errors = [];
  const exits = [];
  let spawned = false;
  run([], {
    platform: 'freebsd',
    arch: 'x64',
    resolve: () => '/never',
    spawn: () => {
      spawned = true;
    },
    exit: (code) => exits.push(code),
    stderr: { write: (s) => errors.push(s) },
  });
  assert.equal(spawned, false);
  assert.deepEqual(exits, [1]);
  assert.match(errors.join(''), /対応表/);
});
