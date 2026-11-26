# 名前に Google の商標を入れず、gmn にする（npm だけ scope 付き）

この fork は google-gemini/gemini-api-cli の非公式の fork で、最初は `gemini-api-plus`、次に `gemini-headless-cli` の名前で公開した。Google の商標ガイドラインは、Gemini などの商標を自分の製品名に組み込むこと（ハイフンでつなぐ形を含む）を禁じていて、説明文に「非公式・Google とは関係ない」と書いても、この禁止は解けない。そこで名前から Google の商標を外し、Gemini の子音だけを残した `gmn` にした（記録は #28）。Gemini には説明文で「Gemini API と使える」「google-gemini/gemini-api-cli の fork」のように平文で触れる。

名前は次のとおり。

- コマンド: `gmn`（`gmn-cli` も同じものを指す）
- PyPI・Homebrew の cask・GitHub の repo: `gmn-cli`
- npm: `@yasuguin/gmn-cli`（OS 別は `@yasuguin/gmn-cli-<os>-<arch>`）。scope なしの `gmn-cli` は、npm が既存の `opn-cli` に似すぎるとして公開を拒否した（E403）。scope 付きの名前は、この類似の判定に当たらない。
- タグ: `gmn-vX.Y.Z`

旧名は npm・PyPI とも削除し、名前を手放した。

## 考えた案

- `gemi`・`gemn` など、Gemini を縮めた名前: 商標を変形した名前に見え、取り違えのおそれが残るので選ばなかった。
- ふたご座の星（`pollux` など）: 商標とは無関係で安全だが、Gemini との関係が名前から伝わらないので選ばなかった。
- npm でも scope なしの名前（`gmncli` など）: 同じ類似の判定で拒否される見込みが高く、公開を試すまで通るか分からないので選ばなかった。
