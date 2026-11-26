# gemini-headless-cli

Unofficial fork of google-gemini/gemini-api-cli. Not affiliated with Google.

`gemini-headless` is the fork's CLI. It is installed side by side with the official `gemini-api` command and does not replace it.

## Usage

```sh
uvx gemini-headless-cli --version
uv tool install gemini-headless-cli
uvx --from gemini-headless-cli gemini-headless --version
```

Each OS and CPU architecture gets its own wheel that contains the native binary, so no Go toolchain or Python runtime dependency is needed beyond Python itself.

Source and issues: https://github.com/yasuguin/gemini-headless-cli
