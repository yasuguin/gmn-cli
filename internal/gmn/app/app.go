// Package app は gmn の起動処理。本家の cli パッケージを呼び出す前に、
// 設定の場所・出力のコマンド名を fork 用に切り替える。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google-gemini/gemini-api-cli/internal/cli"
	"github.com/google-gemini/gemini-api-cli/internal/clierrors"
	"github.com/google-gemini/gemini-api-cli/internal/config"
	"github.com/google-gemini/gemini-api-cli/internal/gmn"
)

// Run は gmn を実行して終了コードを返す。
func Run(args []string, stdout, stderr io.Writer) int {
	gmn.Enable()
	// 本家は PersistentPreRunE と --help 時の認証確認で config.Init("gemini-api", ...) を呼ぶが、
	// Init は最初の 1 回だけ効くので、先に fork 用の名前で呼んで設定の場所とキーチェーンのサービス名を分ける。
	// 環境変数の接頭辞 GEMINI は GEMINI_API_KEY などの互換のため変えない。
	// エラーは保存され、本家側の Init が同じエラーを返す。
	_ = config.Init(gmn.Name, "GEMINI")

	root, err := cli.NewRootCommand()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	// 本家の文面は gemini-api を直書きしているので、出力の手前で置き換える。
	errW := gmn.NewWriter(stderr)
	outW := gmn.NewWriter(stdout)
	root.SetErr(errW)
	if gmn.StdoutIsDocs(root, args) {
		root.SetOut(outW)
	} else {
		root.SetOut(stdout)
	}
	defer errW.Flush()
	defer outW.Flush()

	ctx, stop := cli.InterruptContext(context.Background())
	defer stop()

	if err := cli.ExecuteRoot(ctx, root, args); err != nil {
		var rendered interface{ Rendered() bool }
		if !errors.As(err, &rendered) || !rendered.Rendered() {
			fmt.Fprintln(errW, config.ScrubSecrets(err.Error()))
		}
		return clierrors.ExitCode(err)
	}
	return 0
}
