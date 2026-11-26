# gemini-headless-cli

[日本語](README.ja.md)

A headless CLI for Gemini models, for agents, scripts, and CI — an unofficial fork of [google-gemini/gemini-api-cli](https://github.com/google-gemini/gemini-api-cli) that also targets Agent Platform. Not affiliated with Google.

This is a one-shot command you call from a shell, not the interactive Gemini CLI agent, and not a wrapper around Gemini CLI's headless mode. It does not edit files or run tools on its own; it sends one request to a Gemini model and returns the result.

The command is `gemini-headless`. It runs next to the upstream `gemini-api` command without conflict. Upstream usage is documented in the [upstream README](https://github.com/google-gemini/gemini-api-cli#readme). This README covers only what the fork adds.

## What is different

- Gemini API (default): the same behavior as upstream.
- Agent Platform (formerly Vertex AI): the same commands can call Agent Platform with Application Default Credentials (ADC). Use this when you need IAM, audit logs, or VPC Service Controls and cannot hand out API keys.
- Separate config location and command name, so both CLIs can be installed together.
- Upstream feature request for this: [google-gemini/gemini-api-cli#241](https://github.com/google-gemini/gemini-api-cli/issues/241).

## Install

```bash
npx gemini-headless-cli --help          # or: pnpm dlx gemini-headless-cli --help
uvx gemini-headless-cli --help          # installs gemini-headless and gemini-headless-cli
brew install --cask yasuguin/tap/gemini-headless-cli   # macOS only
curl -fsSL https://raw.githubusercontent.com/yasuguin/gemini-headless-cli/main/scripts/plus/install.sh | bash
```

The install script downloads the binary from [GitHub Releases](https://github.com/yasuguin/gemini-headless-cli/releases) (tags `headless-v*`) and checks it against `checksums.txt`. If the check fails, nothing is installed. It installs to `/usr/local/bin` if writable, otherwise `~/.local/bin`. Set `GEMINI_INSTALL_DIR` to choose the directory, and `GEMINI_VERSION` to pin a version (for example `0.1.0`). Windows is not supported by the script; download the zip from Releases.

## Quick start

### Gemini API

```bash
export GEMINI_API_KEY=...        # or: gemini-headless configure
gemini-headless generate "Write a haiku about APIs"
```

### Agent Platform

```bash
gcloud auth application-default login
export GOOGLE_GENAI_USE_VERTEXAI=true
export GOOGLE_CLOUD_PROJECT=my-project
export GOOGLE_CLOUD_LOCATION=global
gemini-headless whoami
gemini-headless generate "Write a haiku about APIs"
```

Check `whoami` before a paid call. It shows the backend, project, and location, and where each came from.

## Agent Platform settings

Each setting is read in this order, and the first one set wins:

1. Flag: `--backend vertex`, `--project`, `--location`
2. Environment: `GOOGLE_GENAI_USE_VERTEXAI`, `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`
3. Config file `~/.config/gemini-headless/config.yaml`
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
- `gemini-headless models` lists the offline catalog with each model's Agent Platform status.

## Coexistence with upstream

- Command: `gemini-headless` here, `gemini-api` upstream.
- Config: `~/.config/gemini-headless/config.yaml` here, `~/.config/gemini-api/config.yaml` upstream.
- Keychain service name differs.
- Environment variables with the `GEMINI_` prefix (such as `GEMINI_API_KEY`) are shared by both.

## Following upstream

The fork merges upstream releases with `scripts/plus/sync-upstream.sh`. The fork's version number is independent of upstream. `gemini-headless --version` also shows the upstream base version.

## License

Apache-2.0, the same as upstream. See [LICENSE](../LICENSE). Files changed by this fork are marked with a `plus:` comment.
