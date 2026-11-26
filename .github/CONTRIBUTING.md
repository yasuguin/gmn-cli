# gmn-cli の保守

この repo は google-gemini/gemini-api-cli の非公式の fork です。バグ報告と要望は [Issues](https://github.com/yasuguin/gmn-cli/issues) へどうぞ。本家への報告は本家の repo へ。

以下は保守者向けの手順です。npm・PyPI・Homebrew に公開しているパッケージなので、main は PR だけで変えます。

## 変更の入れ方

- main への直接 push・強制 push・削除は、ruleset で拒否されます。ブランチを切って PR を出してください。
- commit は GitHub の noreply アドレスで作ります。clone したら最初に設定してください。
  ```sh
  git config user.email "<ID>+<ユーザー名>@users.noreply.github.com"
  ```
  PR に noreply 以外のアドレスの commit が混ざると、必須チェック `noreply` が落ちます。落ちたときは、出力の直し方に従ってください。
- 通常の PR は squash でマージします。PR の題が main の 1 commit になるので、題は conventional commit の形（`fix(plus): …`・`feat(plus): …`）で書きます。
- 本家の取り込みの PR だけは merge commit でマージします。本家の履歴を残し、次の取り込みの基点を崩さないためです（`scripts/plus/sync-upstream.sh` は rebase しない前提）。
