'use strict';

// 本体パッケージの bin から呼ばれる。OS 別パッケージの実行ファイルを子プロセスで起動し、
// 終了コードとシグナルを呼び出し元へ伝える。
// postinstall を使わないのは pnpm v10 が依存の lifecycle script を既定で止めるため。
const { spawn } = require('node:child_process');
const { resolveBinary } = require('./platform');

// 親が受けたシグナルは子へ転送する。子の終了を待って、親も同じシグナルで終わらせる。
const FORWARDED_SIGNALS = ['SIGINT', 'SIGTERM', 'SIGHUP'];

function run(argv, deps = {}) {
  const platform = deps.platform || process.platform;
  const arch = deps.arch || process.arch;
  const resolve = deps.resolve || require.resolve;
  const spawnFn = deps.spawn || spawn;
  const stderr = deps.stderr || process.stderr;
  const exit = deps.exit || ((code) => process.exit(code));
  const kill = deps.kill || ((sig) => process.kill(process.pid, sig));

  const found = resolveBinary(platform, arch, resolve);
  if (found.error) {
    stderr.write(`${found.error}\n`);
    exit(1);
    return null;
  }

  const child = spawnFn(found.path, argv, { stdio: 'inherit' });
  const forwarders = FORWARDED_SIGNALS.map((sig) => {
    const fn = () => child.kill(sig);
    process.on(sig, fn);
    return [sig, fn];
  });
  const cleanup = () => {
    for (const [sig, fn] of forwarders) process.removeListener(sig, fn);
  };

  child.on('error', (err) => {
    cleanup();
    stderr.write(`gemini-headless: 起動できない: ${err.message}\n`);
    exit(1);
  });
  child.on('exit', (code, signal) => {
    cleanup();
    if (signal) {
      // 子がシグナルで終わったときは同じシグナルで親も終わらせ、終了の理由を呼び出し元に残す。
      kill(signal);
      return;
    }
    exit(code === null ? 1 : code);
  });
  return child;
}

module.exports = { run, FORWARDED_SIGNALS };
