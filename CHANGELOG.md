# Changelog

## 0.2.1 (2026-11-28)

<!-- Release notes generated using configuration in .github/release.yml at main -->



**Full Changelog**: https://github.com/yasuguin/gmn-cli/compare/gmn-v0.2.0...gmn-v0.2.1

## 0.2.0 (2026-11-27)

<!-- Release notes generated using configuration in .github/release.yml at main -->

## What's Changed
### Features
* feat(plus): show where ADC credentials come from in whoami by @yasuguin in https://github.com/yasuguin/gmn-cli/pull/56
### Fixes
* fix(plus): reject tts --multi-speaker entries with extra '=' separators by @yasuguin in https://github.com/yasuguin/gmn-cli/pull/53
* fix(plus): send requests for the us and eu locations to the multi-region endpoints by @yasuguin in https://github.com/yasuguin/gmn-cli/pull/59
* fix(plus): make music work on Agent Platform by @yasuguin in https://github.com/yasuguin/gmn-cli/pull/60


**Full Changelog**: https://github.com/yasuguin/gmn-cli/compare/gmn-v0.1.2-rc.2...gmn-v0.2.0

## 0.1.2-rc.2 (2026-11-26)

npm の公開を trusted publisher（OIDC）に切り替えたことを確かめるためのプレリリース。利用者向けの変更はない。

**Full Changelog**: https://github.com/yasuguin/gmn-cli/compare/gmn-v0.1.2-rc.1...gmn-v0.1.2-rc.2

## 0.1.2-rc.1 (2026-11-26)

release-please を入れてから最初の公開の流れを確かめるためのプレリリース。利用者向けの変更はない。

**Full Changelog**: https://github.com/yasuguin/gmn-cli/compare/gmn-v0.1.1...gmn-v0.1.2-rc.1

## [0.1.1](https://github.com/yasuguin/gmn-cli/compare/gmn-v0.1.0...gmn-v0.1.1) (2026-11-26)

### 配布

- npm の本体を `@yasuguin/gmn-cli` で公開する。scope なしの `gmn-cli` は、npm が既存のパッケージ名に似すぎるとして公開を拒否したため（`npx @yasuguin/gmn-cli`）。PyPI と Homebrew は `gmn-cli` のまま。

## [0.1.0](https://github.com/yasuguin/gmn-cli/releases/tag/gmn-v0.1.0) (2026-11-26)

google-gemini/gemini-api-cli（本家 v0.5.0）の非公式の fork として、`gmn` を最初に公開した版。旧名 `gemini-headless-cli` から改名した（旧名の版は取り下げ済み。経緯は [ADR 0001](docs/adr/0001-name-without-google-trademarks.md)）。

### Agent Platform（旧 Vertex AI）対応

- `--backend vertex`、または `GOOGLE_GENAI_USE_VERTEXAI=true`・`GOOGLE_CLOUD_PROJECT`・`GOOGLE_CLOUD_LOCATION` で、同じコマンドから Agent Platform を呼べる。既定は本家と同じ Gemini API。
- 認証は Application Default Credentials（ADC）。トークンは要求ごとに更新するので、長い処理でも期限切れで落ちない。
- `generate`・`analyze`・`transcribe`・`agent`・`models list`/`get` が Agent Platform で動く。`image` と `tts` は `generateContent` で動く。
- モデル名は両方で同じ名前を使える。Agent Platform に無い既定のモデルは代わりのモデルに読み替え、`--model` で明示したモデルが無ければ候補を示して止まる。
- `analyze`・`transcribe` の入力に `gs://` の URI を渡せる。
- Agent Platform に無いコマンド（`files upload` など）は、送る前に理由を示して止まる。

### 本家と並べて使う

- コマンド `gmn`・設定の置き場所・キーチェーンのサービス名を本家と分け、本家の `gemini-api` と同時に入れられる。
- `gmn --version` に、土台にした本家の版を出す。

### 配布

- GitHub Releases（6 種類の OS・CPU）、Homebrew（`brew install --cask yasuguin/tap/gmn-cli`）、PyPI（`uvx gmn-cli`）、checksum を確かめる install スクリプト。
- npm は OS 別のパッケージだけが出て、本体は 0.1.1 から（上記）。
- エージェント向けの skill を同梱する。
