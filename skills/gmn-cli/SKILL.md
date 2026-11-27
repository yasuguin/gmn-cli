---
name: gmn-cli
description: Call Gemini API or Agent Platform (Vertex AI) from agents, scripts, and CI with the gmn CLI (generate, analyze, transcribe, image, TTS).
---

# gmn

`gmn` is an unofficial fork of the upstream `gemini-api` CLI. It sends the same commands to either:

- the Gemini API (default, authenticated with an API key), or
- Agent Platform (formerly Vertex AI), authenticated with Application Default Credentials (ADC), so IAM, audit logs, and VPC Service Controls apply. Data Access audit logs for `aiplatform.googleapis.com` are off by default, so calls are logged only if the project turns them on. When they are on, both Interactions API and `generateContent` calls are logged with the caller's account.

Run `gmn --help` and `gmn <command> --help` for the full flag list. This skill covers what an agent needs to choose the backend, recover from auth errors, and avoid commands that Agent Platform does not serve.

## 1. Choose and check the backend first

Run this before any real call:

```bash
gmn whoami
```

It prints the resolved `backend`, `project`, and `location`, and where each value came from (flag, env, config, or default). Check that the project is the one you expect before a call that costs money. With `--backend vertex`, the `auth` line also shows which credential ADC will use: the `GOOGLE_APPLICATION_CREDENTIALS` file (with its type and service account), the gcloud ADC file, or the metadata server.

Backend resolution order (first match wins):

1. `--backend vertex|gemini`
2. `GOOGLE_GENAI_USE_VERTEXAI=true|false`
3. `backend:` in `~/.config/gmn/config.yaml`
4. default `gemini`

Project and location use the same order: flag, env (`GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`), config (`project:`, `location:`). Location defaults to `global`.

To use Agent Platform for one call:

```bash
gmn generate "Hello" --backend vertex --project my-project
```

## 2. Authentication

- Gemini API: set `GEMINI_API_KEY`, or run `gmn configure` (the user must create the key; do not create one for them).
- Agent Platform: ADC, in this order: `--access-token` (highest priority), then `GOOGLE_APPLICATION_CREDENTIALS`, the gcloud ADC file, then the metadata server (on Cloud Run, GCE, and Workload Identity). The token is refreshed per request.

When Agent Platform auth fails, the error names the fix. Typical fixes:

- No ADC found: `gcloud auth application-default login`, or set `GOOGLE_APPLICATION_CREDENTIALS`.
- Token could not be refreshed: run `gcloud auth application-default login` again.
- Permission denied (403) although the user is logged in: run `gmn whoami --backend vertex` and read the `auth` line. If it says `GOOGLE_APPLICATION_CREDENTIALS`, that file is used and the gcloud login is ignored, so logging in again does not help. Ask the user whether to unset the variable for this call (`env -u GOOGLE_APPLICATION_CREDENTIALS gmn ...`) or grant that account access.
- No project: pass `--project`, set `GOOGLE_CLOUD_PROJECT`, or add `project:` to the config file.

Do not switch to an API key to get past an Agent Platform error. The CLI does not send an API key to Agent Platform.

## 3. Model names

- Use the same model names on both backends.
- If you do not pass `--model`, and the default is not served on Agent Platform, the CLI uses a replacement and prints a notice (`Using <replacement>: <default> is not available on Agent Platform`) to stderr, except with `-o json` or `--agent-mode`. Example: `transcribe` uses `gemini-3.8-flash` on Agent Platform instead of `gemini-3.5-transcribe`.
- If you pass `--model` explicitly and Agent Platform does not serve it, the CLI stops with a list of alternatives. It does not switch models silently. Pick one of the listed names and pass it again.
- `gmn models` prints the offline catalog with the Agent Platform status of each model. `gmn models list` and `models get` query the live API.

Models verified on Agent Platform (2026-11-26): `gemini-4-hydrogen` (default), `gemini-4-argon`, `gemini-4-carbon`, `gemini-4-neon`, `gemini-3.8-flash`, `gemini-3.7-flash`, `gemini-3.5-flash`. Not served: `gemini-2.5-flash`, `gemini-2.5-pro`, `gemini-flash-latest`, `gemini-pro-latest`, `gemma-4-31b-it`.

## 4. Commands on Agent Platform

Works:

- `generate`, `analyze`, `agent run`, `agent status`, `agent list`
- `transcribe` (default model is replaced as above)
- `image`: writes an image file; `--aspect-ratio`, `--image-size`, `--output-mime-type`, and an input image for edits work
- `tts`: writes a WAV file; `--voice`, `--style`, and `--multi-speaker` (up to 2 speakers) work
- `music`: writes an MP3 file with `lyria-3-pro-preview` (replaces the default `lyria-3.5`, which Agent Platform does not serve) or `lyria-3-clip-preview`
- `video` with the default `gemini-omni-1.2-flash`
- `agent cancel`, `agent delete-interaction`, `agent get`, `agent delete`. Agent Platform rejects `agent create` with the base agent `antigravity-preview-09-2026`.
- `gs://` URIs in `--input` for `analyze` and `transcribe`
- `models list`, `models get`, `whoami`, `models`

Refused before any request, with exit code 2:

- `files` (including `files upload`). Put the file in Cloud Storage and pass its `gs://` URI with `--input`.
- `environments`, `triggers`, `webhooks`, `voices`, `credentials`

Locations: commands that use the Interactions API (all except `image`, `tts`, and `models`) accept only `global`, `us`, and `eu`. A single region such as `us-central1` fails with `Unsupported location`. `image` and `tts` go to the location you pass, but each model is served only in some locations; on a not-found error, retry with `--location global`.

Not supported: `tts --voice-prompt` (custom voices). Use a built-in voice name.

## 5. Command examples

These run against the default backend unless you pass `--backend vertex`.

```bash
gmn generate "Explain concurrency in one sentence"
gmn generate "Write a haiku" --stream=false -o json
gmn analyze -i photo.jpg "What is in this picture?"
gmn analyze -i gs://my-bucket/talk.mp4 "Summarize the talk"
gmn transcribe -i interview.mp3 --format srt --out interview.srt
gmn image "a lighthouse at sunset" --aspect-ratio 16:9 --out lighthouse.png
gmn tts "Welcome to the show" --voice Puck --out welcome.wav
gmn tts "Alice: hi. Bob: hello." --multi-speaker "Alice=Kore,Bob=Puck" --out dialog.wav
gmn models list -o json
gmn agent run --help
```

Use `--help` on the subcommand before you guess a flag. Do not invent flags. Global flags (auth, network, output) are listed only in `gmn --help-global`, not in `gmn <command> --help`.

## 6. Flags for agents

- `--timeout <duration>` (for example `3m`): always pass it from scripts and agents. Without it there is no deadline, and a call that gets no response waits forever. The deadline covers the whole call, including a streamed reply. When it runs out, the CLI exits 1 with `connection_error: context deadline exceeded`. Thinking models can take a while before the first token, so leave room.
- Retries: transient errors such as 503 are retried automatically with backoff, for up to about 30 seconds in total. Do not wrap the CLI in your own retry loop.
- `--stdin` and `--system <text>` on `generate`: read the prompt from stdin and set the system instruction. Use them for long or multi-line text instead of putting it in the positional prompt, for example `gmn generate --system "$(cat instructions.md)" --stdin < draft.md`.
- Output of `generate`: by default it streams only the reply text to stdout and prints the `Interaction ID` to stderr, so `> out.md` captures just the text. `--stream=false` prints the whole interaction object instead (pretty YAML). To get only the text from it, add `--jq '[.steps[] | select(.type=="model_output") | .content[] | select(.type=="text") | .text] | join("")'`.
- `--no-interactive`: never prompt or open a form. Use it in CI and other runs with no one to answer.
- `--dry-run`: builds the request and prints it without sending. Use it to check the URL, the backend, and the body before a paid call. Credentials are masked.
- `--agent-mode`: structured errors with the fix in each error. Use it when you parse failures.
- `-o json` or `--jq '<expr>'`: machine-readable output. Use `--jq` to pick a field.
- `-d` / `--debug`: logs the request and response to stderr. Use it when an error does not explain itself.
- `--usage`: prints the command schema in KDL, for discovering flags.

Exit codes: 0 ok, 1 runtime error, 2 usage error (including a command Agent Platform does not serve), 3 authentication or authorization error.

## 7. Things not to do

- Do not run `configure` or create an API key on the user's behalf.
- Do not pass a local file larger than about 14 MB to `analyze` or `transcribe`. Upload it to Cloud Storage and pass the `gs://` URI on Agent Platform, or use `files upload` on the Gemini API.
