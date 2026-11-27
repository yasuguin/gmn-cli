# gmn-cli のエージェント向けの決まり

この repo は google-gemini/gemini-api-cli の非公式の fork で、public に置き、npm・PyPI・Homebrew へ公開している。main に入ったものがそのまま利用者に届くので、main は PR だけで変える。

保守の手順の正本は [.github/CONTRIBUTING.md](.github/CONTRIBUTING.md)。ここには、エージェントが判断に使う基準だけを置く。

## 機械が止めるもの

ruleset「main は PR だけで変える」（bypass は誰にも無い）と repo の設定が、次を拒否する。

- main への直接 push・強制 push・削除、PR 以外での main の変更
- rebase merge（使えるのは squash と merge commit だけ）
- 必須チェック `branch-name`（`gmn/` などの接頭辞）と `noreply`（commit のアドレス）が落ちた PR のマージ

拒否されたら ruleset や設定を緩めず、出力の直し方に従って commit・ブランチを直す。

## 機械で止めていない判断

- **マージの方法**: 通常の PR は squash。merge commit は本家の取り込みの PR（`upstream-sync/<tag>`）だけに使う。本家の履歴を残して次の取り込みの基点を保つためで、ほかの PR で merge commit を選ぶと版の計算と履歴が崩れる。
- **PR の題**: squash で main の 1 commit になり、release-please がそこから版と変更履歴を決める。conventional commit の形（`fix(plus): …`・`feat(plus): …`）で、利用者に見える変化を書く。
- **リリース PR**（`release-please--…`）: マージすると npm・PyPI・Homebrew へ出て戻せない。変更履歴に「機能」「修正」「本家の取り込み」が載ったときだけ、ユーザーに確かめてからマージする。タグは手で打たない。
- **public に出る文字**: commit・issue・PR・コードに、API key・認証情報・社内の名前や情報を書かない。push protection は既知の形式の key しか止めない。
- **本家への出力**: 本家（google-gemini/gemini-api-cli）に issue・PR を出さない。fork の不具合は fork の Issues に書く。
