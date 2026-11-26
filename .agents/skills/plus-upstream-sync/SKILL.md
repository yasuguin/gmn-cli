---
name: plus-upstream-sync
description: fork（gmn-cli）へ本家 google-gemini/gemini-api-cli の新しいリリースを取り込むとき、merge が衝突したとき、取り込み後の確認をするときに使う。保守者向けで、利用者向けではない。
metadata:
  internal: true
---

# 本家リリースの取り込み（保守者向け）

`metadata.internal: true` で `npx skills add` の一覧から外している。利用者向けの skill は `skills/` に置く。

## いつ使うか

- 週 1 回の workflow `plus-upstream-watch` は、本家の新しいリリースを自動で取り込みの PR（`chore(upstream): 本家 vX.Y.Z を取り込む`）にする。PR の CI が通れば、**merge commit で**マージする（squash しない）。
- 自動の取り込みが衝突したときは、PR の代わりに「upstream vX.Y.Z を取り込む」issue が立つ。そのときは下の手順で手元で取り込み、PR を出す。
- 本家の変更を手元で先に取り込みたいとき。

## 手順

1. 作業ツリーに未コミットの変更が無い状態で、main から取り込みのブランチ（例 `upstream-sync/vX.Y.Z`）を切る。main は PR 必須なので、直接 merge しない。
2. `scripts/plus/sync-upstream.sh --dry-run vX.Y.Z` で予定を見る。
3. `scripts/plus/sync-upstream.sh vX.Y.Z` を実行する。終了コード 0 なら、merge・`plus:` の印の確認・`go build`・`go test` が通っている。
4. 衝突で止まったら、下の基準で直して commit する。

## 衝突の直し方

- 本家の変更を優先し、fork の差し込みは `// plus:` の印を付けて、本家の行の近くに呼び出しの 1 行として付け直す。
- 生成された SDK（`internal/sdk/` 配下、Speakeasy 製）は常に本家側を採る（`git checkout --theirs -- <file>`）。fork の差し込みは手書きの層（`internal/plus/`）へ移す。
- `internal/plus/` 配下は fork 側を採る（`git checkout --ours -- <file>`）。
- `--usage` の事前生成テーブルや help 全走査テストが落ちたら、テストの期待値を書き換えず、fork 側の差し込みの位置と内容を見直す。

衝突を直して commit した後は、script の印の比較は走らない。代わりに次の 2 つを比べる:

```
git grep -h -F '// plus:' HEAD^1 -- . ':(exclude)internal/plus' ':(exclude)scripts/plus' ':(exclude).agents' | sort
git grep -h -F '// plus:' -- . ':(exclude)internal/plus' ':(exclude)scripts/plus' ':(exclude).agents' | sort
```

後の一覧から消えた行があれば、欠けた差し込みを付け直す。

## merge 後に確かめること

- `git tag -l 'v*'` が空（本家のタグは `refs/tags/upstream/` にだけ入る）。`gmn-v*` のタグは変わっていない。
- 上の印の比較で欠けた行が無い。
- `go build ./...` と `go test ./... -count=1` が通る。
