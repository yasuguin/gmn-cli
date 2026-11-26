# gmn-cli

Unofficial fork of google-gemini/gemini-api-cli. Not affiliated with Google.

`gmn` is the fork's CLI. It is installed side by side with the official `gemini-api` command and does not replace it.

## Usage

```sh
uvx gmn-cli --version
uv tool install gmn-cli
uvx --from gmn-cli gmn --version
```

Each OS and CPU architecture gets its own wheel that contains the native binary, so no Go toolchain or Python runtime dependency is needed beyond Python itself.

Source and issues: https://github.com/yasuguin/gmn-cli
