# gmn — Gemini API and Agent Platform from the terminal

[![Release](https://img.shields.io/github/v/release/yasuguin/gmn-cli?style=flat-square)](https://github.com/yasuguin/gmn-cli/releases/latest)
[![License](https://img.shields.io/github/license/yasuguin/gmn-cli?style=flat-square)](../LICENSE)
[![Homebrew](https://img.shields.io/badge/Homebrew-yasuguin%2Ftap-fbb040?style=flat-square&logo=homebrew)](https://github.com/yasuguin/homebrew-tap)

[日本語](README.ja.md)

`gmn` is a one-shot command-line client that works with Gemini models through the Gemini API or Agent Platform (formerly Vertex AI). It is built for agents, scripts, and CI: it sends one request and returns the result. It does not edit files or run tools on its own, and it is not the interactive Gemini CLI agent.

```bash
gmn generate "Write a haiku about APIs"
gmn image "A friendly robot mascot" --out ./assets/
gmn transcribe -i meeting.mp3 --format srt --out meeting.srt
```

`gmn` is an unofficial fork of [google-gemini/gemini-api-cli](https://github.com/google-gemini/gemini-api-cli) and is not affiliated with Google. It runs next to the upstream `gemini-api` command without conflict. Upstream usage is documented in the [upstream README](https://github.com/google-gemini/gemini-api-cli#readme); this README covers only what the fork adds.

## What is different

- Gemini API (default): the same behavior as upstream.
- Agent Platform (formerly Vertex AI): the same commands can call Agent Platform with Application Default Credentials (ADC). Use this when you need IAM, audit logs, or VPC Service Controls and cannot hand out API keys. Upstream has an [open feature request](https://github.com/google-gemini/gemini-api-cli/issues/241) for this.
- Separate config location and command name, so both CLIs can be installed together.

## Install

Pick one. Every option gives you the same `gmn` binary.

### Homebrew (macOS)

```bash
brew install --cask yasuguin/tap/gmn-cli
```

### mise

From GitHub Releases. Needs no Node.js.

```bash
mise use -g github:yasuguin/gmn-cli
```

From npm. Use this when your network blocks GitHub or your CI hits the GitHub API rate limit.

```bash
mise use -g npm:@yasuguin/gmn-cli
```

### npm

Run without installing:

```bash
npx @yasuguin/gmn-cli --help
```

```bash
pnpm dlx @yasuguin/gmn-cli --help
```

Install globally:

```bash
pnpm add -g @yasuguin/gmn-cli
```

### PyPI

Run without installing:

```bash
uvx gmn-cli --help
```

Install as a tool. This provides both the `gmn` and `gmn-cli` commands.

```bash
uv tool install gmn-cli
```

### Install script (macOS and Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/yasuguin/gmn-cli/main/scripts/plus/install.sh | bash
```

The script downloads the binary from [GitHub Releases](https://github.com/yasuguin/gmn-cli/releases) (tags `gmn-v*`) and checks it against `checksums.txt`. If the check fails, nothing is installed. It installs to `/usr/local/bin` if writable, otherwise `~/.local/bin`.

To choose the directory or pin a version, set `GMN_INSTALL_DIR` and `GMN_VERSION`:

```bash
curl -fsSL https://raw.githubusercontent.com/yasuguin/gmn-cli/main/scripts/plus/install.sh | GMN_INSTALL_DIR="$HOME/bin" GMN_VERSION=0.1.0 bash
```

### Windows

The install script does not support Windows. Download the zip from [GitHub Releases](https://github.com/yasuguin/gmn-cli/releases).

## Quick start

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

Check `whoami` before a paid call. It shows the backend, project, and location, and where each came from.

## Use from agents

This repo ships an agent skill ([skills/gmn-cli](../skills/gmn-cli/SKILL.md)) that tells coding agents how to call `gmn`. Install it with the [skills](https://github.com/vercel-labs/skills) CLI:

```bash
npx skills add yasuguin/gmn-cli
```

## Agent Platform settings

Each setting is read in this order, and the first one set wins:

1. Flag: `--backend vertex`, `--project`, `--location`
2. Environment: `GOOGLE_GENAI_USE_VERTEXAI`, `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`
3. Config file `~/.config/gmn/config.yaml`
4. Default: `gemini` backend, location `global`

```yaml
backend: vertex
project: my-project
location: global
```

Authentication with `--backend vertex`:

1. `--access-token`, if given
2. ADC: `GOOGLE_APPLICATION_CREDENTIALS`, the gcloud ADC file, or the metadata server (Cloud Run, GCE, Workload Identity)

The token is refreshed per request, so long runs (video polling, long agent runs) do not fail when it expires. The quota project is set to `--project` unless you set `--user-project`.

The API key is not sent to Agent Platform.

Environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `GEMINI_API_KEY` | none | API key for the Gemini API. Not sent to Agent Platform. |
| `GOOGLE_GENAI_USE_VERTEXAI` | `false` | `true` or `1` selects Agent Platform. |
| `GOOGLE_CLOUD_PROJECT` | none | Google Cloud project for Agent Platform. |
| `GOOGLE_CLOUD_LOCATION` | `global` | Agent Platform location. |
| `GOOGLE_APPLICATION_CREDENTIALS` | none | Credentials file for ADC. Takes priority over the gcloud ADC file. |

## Commands on Agent Platform

Verified with the maintainer's smoke script on 2026-11-26 against the `global` location.

- Works:
  - `generate`, `analyze`, `agent run`, `agent status`, `agent list`
  - `transcribe` (the default model is replaced; see below)
  - `image` through `generateContent`, with 4 models: `gemini-nano-banana-2.1`, `gemini-3.1-flash-image`, `gemini-3-pro-image`, `gemini-2.5-flash-image`. Aspect ratio, size, output MIME type, and image edits work.
  - `tts` through `generateContent`, with 4 models: `gemini-3.8-flash-tts`, `gemini-3.8-flash-lite-tts`, `gemini-3.1-flash-tts-preview`, `gemini-2.5-flash-preview-tts`. `--voice`, `--style`, and `--multi-speaker` (up to 2 speakers) work.
  - `models list`, `models get`
  - Local files in `analyze` and `transcribe` are sent inline if small. Larger media goes through a `gs://` URI.
- Stops before sending (exit code 2):
  - `files` and `files upload`: use a `gs://` URI in `--input`
  - `environments`, `triggers`, `webhooks`, `voices`, `credentials`
  - `tts --voice-prompt` (custom voices)
- Not verified:
  - `music`, `video`
  - `gs://` input in a real call (the smoke run skipped it)
  - `agent` subcommands other than `run`, `status`, `list`
  - locations other than `global`

## Model names

- Use the same model names on both backends.
- With no `--model`, a default that Agent Platform does not serve is replaced, and the replacement is printed to stderr (not with `-o json` or `--agent-mode`). For example, `transcribe` uses `gemini-3.8-flash` instead of `gemini-3.5-transcribe`.
- With an explicit `--model` that Agent Platform does not serve, the command stops and lists alternatives. It does not switch models silently, so a change in cost or quality is visible.
- Verified on Agent Platform (2026-11-26): `gemini-4-hydrogen` (default), `gemini-4-argon`, `gemini-4-carbon`, `gemini-4-neon`, `gemini-3.8-flash`, `gemini-3.7-flash`, `gemini-3.5-flash`.
- Not served: `gemini-2.5-flash`, `gemini-2.5-pro`, `gemini-flash-latest`, `gemini-pro-latest`, `gemma-4-31b-it`.
- `gmn models` lists the offline catalog with each model's Agent Platform status.

## Coexistence with upstream

- Command: `gmn` here, `gemini-api` upstream.
- Config: `~/.config/gmn/config.yaml` here, `~/.config/gemini-api/config.yaml` upstream.
- Keychain service name differs.
- Environment variables with the `GEMINI_` prefix (such as `GEMINI_API_KEY`) are shared by both.

## Following upstream

The fork takes in upstream releases only, not unreleased changes on upstream `main`. Its version number is independent of upstream. `gmn --version` also shows the upstream release it is based on.

## Help

- Bugs and requests: [Issues](https://github.com/yasuguin/gmn-cli/issues) of this fork. Please do not report fork problems upstream.
- Changes: [CHANGELOG](../CHANGELOG.md)
- Security: [SECURITY.md](SECURITY.md)
- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md)

## License

Apache-2.0, the same as upstream. See [LICENSE](../LICENSE). Files changed by this fork are marked with a `plus:` comment.
