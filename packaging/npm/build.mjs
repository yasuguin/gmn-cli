#!/usr/bin/env node
// goreleaser の成果物（アーカイブ 6 つ）から npm の 7 パッケージを組み立てる。
// 本体 1 つと OS 別 6 つで、版はすべて --version に揃える。npm コマンドは使わず、ファイル操作だけをする。
//
// 使い方: node packaging/npm/build.mjs --dist <goreleaser の dist> --out <空の出力先> --version 0.1.0
//
// 出力先には <out>/gmn-cli（本体）と <out>/gmn-cli-<os>-<arch>（OS 別）ができる。
// 公開は release workflow が行う。このスクリプトは何も publish しない。
import { spawnSync } from 'node:child_process';
import { cpSync, chmodSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { SUPPORTED, binaryFileName, packageName } from './lib/platform.js';

const HERE = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(HERE, '..', '..');
const MAIN_NAME = 'gmn-cli';

// goreleaser の name_template に合わせたアーカイブ名（例 gmn-cli_Darwin_arm64.tar.gz）。
const GORELEASER_OS = { darwin: 'Darwin', linux: 'Linux', win32: 'Windows' };
const GORELEASER_ARCH = { x64: 'x86_64', arm64: 'arm64' };

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 2) {
    const key = argv[i];
    if (!key.startsWith('--') || argv[i + 1] === undefined) {
      throw new Error(`引数が不正: ${key}`);
    }
    args[key.slice(2)] = argv[i + 1];
  }
  for (const k of ['dist', 'out', 'version']) {
    if (!args[k]) throw new Error(`--${k} が無い`);
  }
  // 版は npm の semver として通る形だけ許す（例 0.1.0・0.1.0-rc.1）。gmn-v は含めない。
  if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(args.version)) {
    throw new Error(`--version が semver ではない: ${args.version}（gmn-v は外す）`);
  }
  return args;
}

function archivePath(dist, platform, arch) {
  const os = GORELEASER_OS[platform];
  const ext = platform === 'win32' ? 'zip' : 'tar.gz';
  return join(dist, `gmn-cli_${os}_${GORELEASER_ARCH[arch]}.${ext}`);
}

// アーカイブを一時ディレクトリに展開して、実行ファイル 1 つを取り出す。
function extractBinary(archive, binaryName, scratch) {
  const dest = mkdtempSync(join(scratch, 'x-'));
  const result = archive.endsWith('.zip')
    ? spawnSync('unzip', ['-q', archive, '-d', dest], { encoding: 'utf8' })
    : spawnSync('tar', ['-xzf', archive, '-C', dest], { encoding: 'utf8' });
  if (result.status !== 0) {
    throw new Error(`展開に失敗: ${archive}\n${result.stderr || result.error || ''}`);
  }
  const found = findFile(dest, binaryName);
  if (!found) throw new Error(`${archive} の中に ${binaryName} が無い`);
  return found;
}

function findFile(dir, name) {
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) {
      const inner = findFile(p, name);
      if (inner) return inner;
    } else if (entry === name) {
      return p;
    }
  }
  return null;
}

function writeJson(path, value) {
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`);
}

function buildOsPackage(entry, { dist, out, version, scratch, license, readme }) {
  const name = packageName(entry.platform, entry.arch);
  const binName = binaryFileName(entry.platform);
  const dir = join(out, `gmn-cli-${entry.pkg}`);
  mkdirSync(dir, { recursive: true });

  const src = extractBinary(archivePath(dist, entry.platform, entry.arch), binName, scratch);
  cpSync(src, join(dir, binName));
  chmodSync(join(dir, binName), 0o755);
  cpSync(license, join(dir, 'LICENSE'));
  cpSync(readme, join(dir, 'README.md'));

  writeJson(join(dir, 'package.json'), {
    name,
    version,
    description: 'Native binary of gmn-cli (unofficial fork of google-gemini/gemini-api-cli). Not affiliated with Google.',
    license: 'Apache-2.0',
    repository: { type: 'git', url: 'git+https://github.com/yasuguin/gmn-cli.git' },
    // os・cpu が一致する環境にだけ入る。ほかの環境では本体の optionalDependencies から外れる。
    os: [entry.platform],
    cpu: [entry.arch],
    files: [binName, 'README.md', 'LICENSE'],
    publishConfig: { access: 'public' },
  });
  return dir;
}

function buildMainPackage(optionalNames, { out, version, license, readme }) {
  const dir = join(out, MAIN_NAME);
  mkdirSync(dir, { recursive: true });
  cpSync(join(HERE, 'bin'), join(dir, 'bin'), { recursive: true });
  cpSync(join(HERE, 'lib'), join(dir, 'lib'), { recursive: true });
  cpSync(license, join(dir, 'LICENSE'));
  cpSync(readme, join(dir, 'README.md'));
  // 利用者向け skill は正本 skills/gmn-cli/ からコピーする。手で二重管理しない。
  // skills CLI の experimental_sync は本体の skills/<name>/SKILL.md を見つけるので、この位置に置く。
  cpSync(join(ROOT, 'skills', 'gmn-cli'), join(dir, 'skills', 'gmn-cli'), { recursive: true });

  const template = JSON.parse(readFileSync(join(HERE, 'package.json'), 'utf8'));
  const optionalDependencies = Object.fromEntries(optionalNames.map((n) => [n, version]));
  writeJson(join(dir, 'package.json'), { ...template, version, optionalDependencies });
  return dir;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const dist = resolve(args.dist);
  const out = resolve(args.out);
  if (existsSync(out) && readdirSync(out).length > 0) {
    throw new Error(`出力先が空ではない: ${out}`);
  }
  mkdirSync(out, { recursive: true });
  const scratch = mkdtempSync(join(tmpdir(), 'gmn-npm-'));
  const common = {
    dist,
    out,
    version: args.version,
    scratch,
    license: join(ROOT, 'LICENSE'),
    readme: join(HERE, 'README.md'),
  };

  try {
    const built = SUPPORTED.map((entry) => buildOsPackage(entry, common));
    const names = SUPPORTED.map((e) => packageName(e.platform, e.arch));
    const main = buildMainPackage(names, common);
    for (const dir of [...built, main]) console.log(dir);
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

main();
