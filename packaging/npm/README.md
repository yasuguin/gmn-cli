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

## 日本語

エージェント・スクリプト・CI 向けの、google-gemini/gemini-api-cli の非公式の fork です。Google とは関係ありません。

`gmn` コマンドがこの fork の CLI です。公式の `gemini-api` コマンドとは別に入り、置き換えません。

使い方は上の Usage のとおりです。このパッケージに install script はありません。OS と CPU に合うネイティブのバイナリは、`optionalDependencies` にある `@yasuguin/gmn-cli-<os>-<arch>` のどれかから入ります。

対応: darwin-arm64、darwin-x64、linux-arm64、linux-x64、win32-arm64、win32-x64。

ソースと Issues: https://github.com/yasuguin/gmn-cli
