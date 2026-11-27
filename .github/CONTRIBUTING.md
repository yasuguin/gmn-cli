# gmn-cli の保守

この repo は google-gemini/gemini-api-cli の非公式の fork です。バグ報告と要望は [Issues](https://github.com/yasuguin/gmn-cli/issues) へどうぞ。本家への報告は本家の repo へ。

以下は保守者向けの手順です。npm・PyPI・Homebrew に公開しているパッケージなので、main は PR だけで変えます。

## 変更の入れ方

- main への直接 push・強制 push・削除は、ruleset で拒否されます。ブランチを切って PR を出してください。
- ブランチ名は `gmn/<内容>`（例: `gmn/next-0.1.2`）にします。`plus` は fork の差し込みの層（`internal/plus/`・`scripts/plus/`・commit の scope）の名前なので、ブランチには使いません。自動で作られる `release-please--…`・`upstream-sync/<tag>`・`dependabot/…` はそのままです。ほかの名前の PR は、必須チェック `branch-name` が落ちます。
- commit は GitHub の noreply アドレスで作ります。clone したら最初に設定してください。
  ```sh
  git config user.email "<ID>+<ユーザー名>@users.noreply.github.com"
  ```
  PR に noreply 以外のアドレスの commit が混ざると、必須チェック `noreply` が落ちます。落ちたときは、出力の直し方に従ってください。
- 通常の PR は squash でマージします。PR の題が main の 1 commit になるので、題は conventional commit の形（`fix(plus): …`・`feat(plus): …`）で書きます。
- 本家の取り込みの PR だけは merge commit でマージします。本家の履歴を残し、次の取り込みの基点を崩さないためです（`scripts/plus/sync-upstream.sh` は rebase しない前提）。取り込みの PR は、週 1 回の workflow `plus upstream watch` が GitHub App `gmn-cli-bot` で自動で作ります。衝突したときは issue になるので、`.agents/skills/plus-upstream-sync/SKILL.md` の手順で手元で取り込みます。

- Dependabot は脆弱性の修正の PR だけを出します（`.github/dependabot.yml`）。Go の依存は `fix(deps): …` で patch を上げ、GitHub Actions は `ci(deps): …` で版を上げません。脆弱性の報告の受け方は [SECURITY.md](SECURITY.md)。

## リリースの出し方

- タグは手で打ちません。main に PR が入るたびに、release-please がリリース PR（版上げと `CHANGELOG.md` の更新）を作り直します。
- リリース PR は、`ci`・`docs` しか入っていなくても常に 1 つ開いています（次の版の下書き）。変更履歴に「機能」「修正」「本家の取り込み」が載ったときだけマージして公開します。中身の無い版は出しません。
- 版は、前のリリースから main に入った commit の conventional commit の種類で決まります。`fix` で patch、`feat` で minor が上がり、`docs`・`ci`・`chore` だけなら上がりません。0.x の間は、破壊的変更（`feat!` など）も minor の上げにとどめます。
- 本家の取り込みは merge commit で入るので、本家の commit（`feat(cli): …` など）も版の計算に入ります。本家の機能が入ったら minor が上がる、と考えてください。
- `CHANGELOG.md` と GitHub Release の本文は、PR 単位の変更履歴です（GitHub のリリースノートの生成を使う）。PR の題から自動で付くラベルで「機能」（feat）・「修正」（fix）・「本家の取り込み」（chore(upstream)）に分かれ、それ以外（ci・docs・refactor など）は載りません。本家の取り込みは、取り込みの PR 1 つ分の 1 行になります。
- リリース PR をマージすると、`gmn-vX.Y.Z` のタグと下書きの GitHub Release ができ、同じ workflow（`release-gmn.yaml`）が asset を足してから Release を公開し、Homebrew・PyPI・npm に出します。
- プレリリース（例 `0.1.2-rc.1`）を出すときは、`release-please-config.json` の package `"."` に `"release-as": "0.1.2-rc.1"` を足す PR を入れ、できたリリース PR をマージします。出したら、`release-as` を消す PR を入れてください（残すと次のリリース PR も同じ版になる）。プレリリースは npm では `next`、GitHub Release では prerelease（latest にはしない）になり、Homebrew には出ません。
- プレリリースの後の正式版も、`release-as` で版（例 `0.1.2`）を指定して出します。release-please はプレリリースの後、`0.1.3-rc.2` のように接尾辞を残した版を提案するためです。
- リリース PR は GitHub App `gmn-cli-bot` が作ります。既定の `GITHUB_TOKEN` で作った PR では、必須チェック `noreply` が起動しないためです。
