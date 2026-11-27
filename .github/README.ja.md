# gmn — ターミナルから Gemini API と Agent Platform を使う

[![Release](https://img.shields.io/github/v/release/yasuguin/gmn-cli?style=flat-square)](https://github.com/yasuguin/gmn-cli/releases/latest)
[![License](https://img.shields.io/github/license/yasuguin/gmn-cli?style=flat-square)](../LICENSE)
[![Homebrew](https://img.shields.io/badge/Homebrew-yasuguin%2Ftap-fbb040?style=flat-square&logo=homebrew)](https://github.com/yasuguin/homebrew-tap)

[English](README.md)

`gmn` は、Gemini API または Agent Platform（旧 Vertex AI）経由で Gemini のモデルを呼ぶ、ワンショットのコマンドラインクライアントです。エージェント、スクリプト、CI から使う前提で作っていて、リクエストを 1 回送って結果を返します。自分でファイルを編集したりツールを実行したりはせず、対話型の Gemini CLI エージェントでもありません。

```bash
gmn generate "Write a haiku about APIs"
gmn image "A friendly robot mascot" --out ./assets/
gmn transcribe -i meeting.mp3 --format srt --out meeting.srt
```

`gmn` は [google-gemini/gemini-api-cli](https://github.com/google-gemini/gemini-api-cli) の非公式の fork で、Google とは関係ありません。upstream の `gemini-api` コマンドと競合せず併用できます。基本的な使い方は [upstream の README](https://github.com/google-gemini/gemini-api-cli#readme) を参照してください。この README では、本 fork で追加された点のみを説明します。

## 違い

- Gemini API（デフォルト）: upstream と同じ動作です。
- Agent Platform（旧 Vertex AI）: Application Default Credentials (ADC) を使って、同じコマンドから Agent Platform を呼び出せます。IAM、監査ログ、VPC Service Controls が必要で、API キーを配布できない場合に使用してください。upstream にも同じ[機能要望](https://github.com/google-gemini/gemini-api-cli/issues/241)が出ています。
- 設定ファイルの場所とコマンド名が分かれているため、両方の CLI を同時にインストールできます。

## インストール

どれか 1 つを選んでください。どの方法でも同じ `gmn` のバイナリを使います。

### Homebrew（macOS）

```bash
brew install --cask yasuguin/tap/gmn-cli
```

### mise

GitHub Releases から入れます。Node.js は要りません。

```bash
mise use -g github:yasuguin/gmn-cli
```

npm から入れます。GitHub への通信が止められたネットワークや、GitHub API の回数の上限に当たる CI で使えます。

```bash
mise use -g npm:@yasuguin/gmn-cli
```

### npm

入れずに実行する:

```bash
npx @yasuguin/gmn-cli --help
```

```bash
pnpm dlx @yasuguin/gmn-cli --help
```

グローバルに入れる:

```bash
pnpm add -g @yasuguin/gmn-cli
```

### PyPI

入れずに実行する:

```bash
uvx gmn-cli --help
```

ツールとして入れる。`gmn` と `gmn-cli` の 2 つのコマンドが使えます。

```bash
uv tool install gmn-cli
```

### インストールスクリプト（macOS・Linux）

```bash
curl -fsSL https://raw.githubusercontent.com/yasuguin/gmn-cli/main/scripts/plus/install.sh | bash
```

[GitHub Releases](https://github.com/yasuguin/gmn-cli/releases)（タグ `gmn-v*`）からバイナリをダウンロードし、`checksums.txt` と照合します。照合に失敗したら何も入れません。書き込めれば `/usr/local/bin` に、書き込めなければ `~/.local/bin` に入れます。

入れる場所や版を決めるときは、`GMN_INSTALL_DIR` と `GMN_VERSION` を渡します。

```bash
curl -fsSL https://raw.githubusercontent.com/yasuguin/gmn-cli/main/scripts/plus/install.sh | GMN_INSTALL_DIR="$HOME/bin" GMN_VERSION=0.1.0 bash
```

### Windows

インストールスクリプトは Windows に対応していません。[GitHub Releases](https://github.com/yasuguin/gmn-cli/releases) から zip をダウンロードしてください。

## クイックスタート

### Gemini API

```bash
export GEMINI_API_KEY=...        # or: gmn configure
gmn generate "Write a haiku about APIs"
```

### Agent Platform

```bash
gcloud auth application-default login
export GOOGLE_GENAI_USE_VERTEXAI=true
export GOOGLE_CLOUD_PROJECT=my-project
export GOOGLE_CLOUD_LOCATION=global
gmn whoami
gmn generate "Write a haiku about APIs"
```

課金が発生する呼び出しの前に `whoami` で確認してください。バックエンド、プロジェクト、ロケーションと、それぞれの参照元が表示されます。

## エージェントから使う

コーディングエージェントに `gmn` の呼び方を教える agent skill（[skills/gmn-cli](../skills/gmn-cli/SKILL.md)）を同梱しています。[skills](https://github.com/vercel-labs/skills) CLI で入れられます。

```bash
npx skills add yasuguin/gmn-cli
```

## Agent Platform の設定

各設定は以下の順序で読み込まれ、最初に設定されていたものが優先されます。

1. フラグ: `--backend vertex`, `--project`, `--location`
2. 環境変数: `GOOGLE_GENAI_USE_VERTEXAI`, `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`
3. 設定ファイル: `~/.config/gmn/config.yaml`
4. デフォルト: `gemini` バックエンド、ロケーション `global`

```yaml
backend: vertex
project: my-project
location: global
```

`--backend vertex` 指定時の認証順序は以下のとおりです。

1. `--access-token`（指定されている場合）
2. ADC: `GOOGLE_APPLICATION_CREDENTIALS`、gcloud の ADC ファイル、またはメタデータサーバー（Cloud Run、GCE、Workload Identity）

トークンはリクエストごとに更新されるため、長時間の実行（動画のポーリングやエージェントの長時間実行など）でも期限切れで失敗しません。クォータプロジェクトには、`--user-project` を指定しない限り `--project` の値が使われます。

API キーは Agent Platform には送信されません。

環境変数:

| 変数 | 既定値 | 説明 |
| --- | --- | --- |
| `GEMINI_API_KEY` | なし | Gemini API の API キー。Agent Platform には送信されません。 |
| `GOOGLE_GENAI_USE_VERTEXAI` | `false` | `true` か `1` で Agent Platform を使います。 |
| `GOOGLE_CLOUD_PROJECT` | なし | Agent Platform で使う Google Cloud のプロジェクト。 |
| `GOOGLE_CLOUD_LOCATION` | `global` | Agent Platform のロケーション。 |
| `GOOGLE_APPLICATION_CREDENTIALS` | なし | ADC の認証情報ファイル。gcloud の ADC ファイルより優先されます。 |

## Agent Platform でのコマンド対応状況

2026-11-26 にメンテナのスモークスクリプトで `global` ロケーションに対して検証した結果です。

- 動作するもの:
  - `generate`, `analyze`, `agent run`, `agent status`, `agent list`
  - `transcribe`（デフォルトモデルは置き換えられます。後述）
  - `generateContent` 経由の `image`（対応 4 モデル: `gemini-nano-banana-2.1`, `gemini-3.1-flash-image`, `gemini-3-pro-image`, `gemini-2.5-flash-image`）。アスペクト比、サイズ、出力 MIME タイプ、画像編集が動作します。
  - `generateContent` 経由の `tts`（対応 4 モデル: `gemini-3.8-flash-tts`, `gemini-3.8-flash-lite-tts`, `gemini-3.1-flash-tts-preview`, `gemini-2.5-flash-preview-tts`）。`--voice`、`--style`、`--multi-speaker`（最大 2 話者）が動作します。
  - `models list`, `models get`
  - `analyze` と `transcribe` のローカルファイルは、サイズが小さければインラインで送信されます。大きいメディアは `gs://` URI 経由で扱います。
- 送信前に停止するもの（終了コード 2）:
  - `files` および `files upload`: `--input` に `gs://` URI を指定してください
  - `environments`, `triggers`, `webhooks`, `voices`, `credentials`
  - `tts --voice-prompt`（カスタム音声）
- 未検証:
  - `music`, `video`
  - 実際の呼び出しでの `gs://` 入力（スモークテストではスキップ）
  - `run`, `status`, `list` 以外の `agent` サブコマンド
  - `global` 以外のロケーション

## モデル名

- 両方のバックエンドで同じモデル名を使用します。
- `--model` 未指定時、デフォルトモデルが Agent Platform で提供されていない場合は別モデルに置き換えられ、その旨が標準エラー出力（stderr）に表示されます（`-o json` と `--agent-mode` では表示されません）。たとえば `transcribe` では `gemini-3.5-transcribe` の代わりに `gemini-3.8-flash` が使われます。
- Agent Platform で提供されていないモデルを `--model` で明示的に指定した場合、コマンドは停止し、代替モデルの一覧を表示します。コストや品質の変化に気づけるよう、暗黙的なモデルの切り替えは行いません。
- Agent Platform で動作確認済み（2026-11-26）: `gemini-4-hydrogen`（デフォルト）, `gemini-4-argon`, `gemini-4-carbon`, `gemini-4-neon`, `gemini-3.8-flash`, `gemini-3.7-flash`, `gemini-3.5-flash`
- 未提供: `gemini-2.5-flash`, `gemini-2.5-pro`, `gemini-flash-latest`, `gemini-pro-latest`, `gemma-4-31b-it`
- `gmn models` で、各モデルの Agent Platform 対応状況を含むオフラインカタログを一覧表示できます。

## upstream との共存

- コマンド: 本ツールは `gmn`、upstream は `gemini-api` です。
- 設定ファイル: 本ツールは `~/.config/gmn/config.yaml`、upstream は `~/.config/gemini-api/config.yaml` です。
- キーチェーンのサービス名も異なります。
- `GEMINI_` プレフィックスの環境変数（`GEMINI_API_KEY` など）は両方で共有されます。

## upstream への追従

本 fork は upstream のリリースだけを取り込み、upstream の `main` にある未リリースの変更は取り込みません。fork のバージョン番号は upstream とは独立しています。`gmn --version` で、ベースにした upstream のリリースも確認できます。

## 問い合わせ

- 不具合と要望: この fork の [Issues](https://github.com/yasuguin/gmn-cli/issues)。fork の問題を upstream に報告しないでください。
- 変更履歴: [CHANGELOG](../CHANGELOG.md)
- セキュリティ: [SECURITY.md](SECURITY.md)
- 開発への参加: [CONTRIBUTING.md](CONTRIBUTING.md)

## ライセンス

upstream と同じく Apache-2.0 です。[LICENSE](../LICENSE) を参照してください。本 fork で変更されたファイルには `plus:` コメントが付いています。
