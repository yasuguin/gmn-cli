'use strict';

// OS 別パッケージの対応表。goreleaser の 6 種（linux・windows・darwin × amd64・arm64）と一致させる。
// 表の順に並べる（エラーメッセージの対応表の並びになる）。
const SUPPORTED = [
  { platform: 'darwin', arch: 'arm64', pkg: 'darwin-arm64' },
  { platform: 'darwin', arch: 'x64', pkg: 'darwin-x64' },
  { platform: 'linux', arch: 'arm64', pkg: 'linux-arm64' },
  { platform: 'linux', arch: 'x64', pkg: 'linux-x64' },
  { platform: 'win32', arch: 'arm64', pkg: 'win32-arm64' },
  { platform: 'win32', arch: 'x64', pkg: 'win32-x64' },
];

const SCOPE = '@yasuguin/gmn-cli';

function packageName(platform, arch) {
  const entry = SUPPORTED.find((e) => e.platform === platform && e.arch === arch);
  return entry ? `${SCOPE}-${entry.pkg}` : null;
}

function binaryFileName(platform) {
  return platform === 'win32' ? 'gmn.exe' : 'gmn';
}

// Node の require.resolve で OS 別パッケージの場所を探す。入っていなければ null を返す。
function resolveBinary(platform, arch, resolve = require.resolve) {
  const name = packageName(platform, arch);
  if (name === null) {
    return { error: unsupportedMessage(platform, arch) };
  }
  const file = binaryFileName(platform);
  try {
    return { path: resolve(`${name}/${file}`) };
  } catch (err) {
    if (err && err.code === 'MODULE_NOT_FOUND') {
      return {
        error:
          `gmn: ${name} が入っていない。` +
          ' optionalDependencies の取得が止められていないか（--no-optional など）確かめ、' +
          'npm・pnpm・yarn で入れ直す。',
      };
    }
    throw err;
  }
}

function unsupportedMessage(platform, arch) {
  const lines = SUPPORTED.map((e) => `  ${e.platform}-${e.arch}`);
  return [
    `gmn: ${platform}-${arch} は対応していない。対応表:`,
    ...lines,
    'ほかの環境では、GitHub Releases の tar.gz・zip か PyPI 版（gmn-cli）を使う。',
  ].join('\n');
}

module.exports = { SUPPORTED, packageName, binaryFileName, resolveBinary, unsupportedMessage };
