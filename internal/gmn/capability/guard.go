package capability

import (
	"fmt"

	"github.com/google-gemini/gemini-api-cli/internal/clierrors"
	"github.com/google-gemini/gemini-api-cli/internal/gmn"
	"github.com/google-gemini/gemini-api-cli/internal/gmn/backend"
	"github.com/spf13/cobra"
)

// Guard は Agent Platform で使えないコマンドを、通信する前に止める。
// 本家の PersistentPreRunE の末尾（backend.Apply の前）から呼ぶ。
// Apply より前に置くのは、project 未設定のエラーや #5 の認証処理より先に
// 「このコマンドは使えない」と伝えるため。このとき backend.Current() はまだ無いので Resolve で解決する。
func Guard(cmd *cobra.Command) error {
	if !gmn.Enabled() {
		return nil
	}
	s, err := backend.Resolve(cmd)
	if err != nil || !s.IsVertex() {
		return nil
	}
	path := PathOf(cmd)
	entry, _ := Lookup(path)
	switch entry.Kind {
	case Unsupported:
		return newError(path, entry.Note, nil)
	case GCS:
		return newError(path, entry.Note, []string{
			"Put the file in Cloud Storage and pass its gs:// URI with --input (for example: gemini-api analyze --input gs://BUCKET/FILE \"summarize this\")",
		})
	}
	return nil
}

// unsupportedError は CLI_VALIDATION（validation_error）の構造化エラー。
// 本家は終了コードを error_type から決め、unsupported_error は 1（runtime）になる。
// 呼び方の誤り（この接続先では使えないコマンドを選んだ）なので、終了コード 2 になる validation にしている。
type unsupportedError struct {
	msg   string
	hints []string
}

func newError(path, note string, extra []string) error {
	e := &unsupportedError{
		msg: fmt.Sprintf("%q is not available on Agent Platform: %s", path, note),
		hints: append(append([]string{}, extra...),
			"To use this command, switch to the Gemini API: pass --backend gemini (or set GOOGLE_GENAI_USE_VERTEXAI=false)",
			"Run \"gemini-api whoami\" to see the resolved backend, project and location",
		),
	}
	return clierrors.WithExitCode(e, clierrors.ExitUsage)
}

func (e *unsupportedError) Error() string { return e.msg }

func (e *unsupportedError) CLIReason() string { return "CLI_VALIDATION" }

func (e *unsupportedError) CLIHints() []string { return e.hints }

// CLIOmitTypeHints は「この build では使えない」という既定の案内を出さない。理由と直し方は hints に入れてある。
func (e *unsupportedError) CLIOmitTypeHints() bool { return true }
