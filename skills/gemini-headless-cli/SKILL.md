---
name: gemini-headless-cli
description: Call Gemini API or Agent Platform (Vertex AI) headlessly from agents, scripts, and CI with the gemini-headless CLI (generate, analyze, transcribe, image, TTS).
---

# gemini-headless

`gemini-headless` is an unofficial fork of the upstream `gemini-api` CLI. It sends the same commands to either:

- the Gemini API (default, authenticated with an API key), or
- Agent Platform (formerly Vertex AI), authenticated with Application Default Credentials (ADC), so IAM, audit logs, and VPC Service Controls apply.

Run `gemini-headless --help` and `gemini-headless <command> --help` for the full flag list. This skill covers what an agent needs to choose the backend, recover from auth errors, and avoid commands that Agent Platform does not serve.

## 1. Choose and check the backend first

Run this before any real call:

```bash
gemini-headless whoami
```

It prints the resolved `backend`, `project`, and `location`, and where each value came from (flag, env, config, or default). Check that the project is the one you expect before a call that costs money.

Backend resolution order (first match wins):

1. `--backend vertex|gemini`
2. `GOOGLE_GENAI_USE_VERTEXAI=true|false`
3. `backend:` in `~/.config/gemini-headless/config.yaml`
4. default `gemini`

Project and location use the same order: flag, env (`GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`), config (`project:`, `location:`). Location defaults to `global`.

To use Agent Platform for one call:

```bash
gemini-headless generate "Hello" --backend vertex --project my-project
```

## 2. Authentication

- Gemini API: set `GEMINI_API_KEY`, or run `gemini-headless configure` (the user must create the key; do not create one for them).
- Agent Platform: ADC, in this order: `--access-token` (highest priority), then `GOOGLE_APPLICATION_CREDENTIALS`, the gcloud ADC file, then the metadata server (on Cloud Run, GCE, and Workload Identity). The token is refreshed per request.

When Agent Platform auth fails, the error names the fix. Typical fixes:

- No ADC found: `gcloud auth application-default login`, or set `GOOGLE_APPLICATION_CREDENTIALS`.
- Token could not be refreshed: run `gcloud auth application-default login` again.
- No project: pass `--project`, set `GOOGLE_CLOUD_PROJECT`, or add `project:` to the config file.

Do not switch to an API key to get past an Agent Platform error. The CLI does not send an API key to Agent Platform.

## 3. Model names

- Use the same model names on both backends.
- If you do not pass `--model`, and the default is not served on Agent Platform, the CLI uses a replacement and prints the name it used to stderr. Example: `transcribe` uses `gemini-3.8-flash` on Agent Platform instead of `gemini-3.5-transcribe`.
- If you pass `--model` explicitly and Agent Platform does not serve it, the CLI stops with a list of alternatives. It does not switch models silently. Pick one of the listed names and pass it again.
- `gemini-headless models` prints the offline catalog with the Agent Platform status of each model. `gemini-headless models list` and `models get` query the live API.

Models verified on Agent Platform (2026-11-26): `gemini-4-hydrogen` (default), `gemini-4-argon`, `gemini-4-carbon`, `gemini-4-neon`, `gemini-3.8-flash`, `gemini-3.7-flash`, `gemini-3.5-flash`. Not served: `gemini-2.5-flash`, `gemini-2.5-pro`, `gemini-flash-latest`, `gemini-pro-latest`, `gemma-4-31b-it`.

## 4. Commands on Agent Platform

Works:

- `generate`, `analyze`, `agent run`, `agent status`, `agent list`
- `transcribe` (default model is replaced as above)
- `image`: writes an image file; `--aspect-ratio`, `--image-size`, `--output-mime-type`, and an input image for edits work
- `tts`: writes a WAV file; `--voice`, `--style`, and `--multi-speaker` (up to 2 speakers) work
- `models list`, `models get`, `whoami`, `models`

Refused before any request, with exit code 2:

- `files` (including `files upload`). Put the file in Cloud Storage and pass its `gs://` URI with `--input`.
- `environments`, `triggers`, `webhooks`, `voices`, `credentials`

Not verified on Agent Platform: `music`, `video`, `gs://` input (the smoke run skipped it), and `agent` subcommands other than run, status, and list. Treat these as untested and check the output carefully.

Not supported: `tts --voice-prompt` (custom voices). Use a built-in voice name.

## 5. Command examples

These run against the default backend unless you pass `--backend vertex`.

```bash
gemini-headless generate "Explain concurrency in one sentence"
gemini-headless generate "Write a haiku" --stream=false -o json
gemini-headless analyze -i photo.jpg "What is in this picture?"
gemini-headless analyze -i gs://my-bucket/talk.mp4 "Summarize the talk"
gemini-headless transcribe -i interview.mp3 --format srt --out interview.srt
gemini-headless image "a lighthouse at sunset" --aspect-ratio 16:9 --out lighthouse.png
gemini-headless tts "Welcome to the show" --voice Puck --out welcome.wav
gemini-headless tts "Alice: hi. Bob: hello." --multi-speaker "Alice=Kore,Bob=Puck" --out dialog.wav
gemini-headless models list -o json
gemini-headless agent run --help
```

Use `--help` on the subcommand before you guess a flag. Do not invent flags.

## 6. Flags for agents

- `--dry-run`: builds the request and prints it without sending. Use it to check the URL, the backend, and the body before a paid call. Credentials are masked.
- `--agent-mode`: structured errors with the fix in each error. Use it when you parse failures.
- `-o json` or `--jq '<expr>'`: machine-readable output. Use `--jq` to pick a field.
- `--usage`: prints the command schema in KDL, for discovering flags.

Exit codes: 0 ok, 1 runtime error, 2 usage error (including a command Agent Platform does not serve), 3 authentication or authorization error.

## 7. Things not to do

- Do not run `configure` or create an API key on the user's behalf.
- Do not pass a local file larger than about 14 MB to `analyze` or `transcribe`. Upload it to Cloud Storage and pass the `gs://` URI on Agent Platform, or use `files upload` on the Gemini API.
