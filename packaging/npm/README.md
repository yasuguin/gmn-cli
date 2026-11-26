# gemini-headless-cli

Unofficial fork of google-gemini/gemini-api-cli for headless use. Not affiliated with Google.

The `gemini-headless` command is the fork's CLI. It is installed side by side with the official `gemini-api` command and does not replace it.

## Usage

```sh
npx gemini-headless-cli --version
pnpm dlx gemini-headless-cli --version
npm install -g gemini-headless-cli && gemini-headless --version
```

This package has no install scripts. The native binary for your OS and CPU comes from one of the `@yasuguin/gemini-headless-cli-<os>-<arch>` packages listed in `optionalDependencies`.

Supported: darwin-arm64, darwin-x64, linux-arm64, linux-x64, win32-arm64, win32-x64.

Source and issues: https://github.com/yasuguin/gemini-headless-cli
