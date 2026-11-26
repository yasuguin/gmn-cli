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

## リリースの出し方

- タグは手で打ちません。main に PR が入るたびに、release-please がリリース PR（版上げと `CHANGELOG.md` の更新）を作り直します。
- 版は main に入った PR の題（conventional commit）から決まります。`fix` で patch、`feat` で minor が上がります。`docs`・`ci`・`chore` だけなら版は上がりません。
- `CHANGELOG.md` と GitHub Release の本文は、PR 単位の変更履歴です（GitHub のリリースノートの生成を使う）。本家の取り込みは、取り込みの PR 1 つ分の 1 行になります。
- リリース PR をマージすると、`gmn-vX.Y.Z` のタグと GitHub Release ができ、同じ workflow（`release-gmn.yaml`）が GitHub Releases・Homebrew・PyPI・npm に公開します。
- プレリリース（例 `0.1.2-rc.1`）を出すときは、Actions の「Release gmn-cli」を手動で実行し、`release-as` に版を入れます。できたリリース PR を、ほかの PR を main に入れる前にマージしてください（main に PR が入ると、リリース PR は commit から決めた版で作り直されます）。プレリリースは npm では `next`、GitHub Release では prerelease になり、Homebrew には出ません。
- リリース PR と本家の取り込みの PR は、GitHub App `gmn-cli-bot` が作ります（既定の `GITHUB_TOKEN` で作った PR では必須チェックが起動しないため）。
