# gmn-cli

Unofficial fork of google-gemini/gemini-api-cli for agents, scripts, and CI. Not affiliated with Google.

The `gmn` command is the fork's CLI. It is installed side by side with the official `gemini-api` command and does not replace it.

## Usage

```sh
npx @yasuguin/gmn-cli --version
pnpm dlx @yasuguin/gmn-cli --version
npm install -g @yasuguin/gmn-cli && gmn --version
```

This package has no install scripts. The native binary for your OS and CPU comes from one of the `@yasuguin/gmn-cli-<os>-<arch>` packages listed in `optionalDependencies`.

Supported: darwin-arm64, darwin-x64, linux-arm64, linux-x64, win32-arm64, win32-x64.

Source and issues: https://github.com/yasuguin/gmn-cli
