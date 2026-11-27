// gmn: Agent Platform の gs:// 入力と、inline 上限超えの案内（fork 独自）。本家の media.go からは呼ぶだけ。

package custom

import (
	"fmt"

	"github.com/google-gemini/gemini-api-cli/internal/gmn"
	"github.com/google-gemini/gemini-api-cli/internal/gmn/backend"
	"github.com/google-gemini/gemini-api-cli/internal/gmn/gcsinput"
	"github.com/spf13/cobra"
)

// onAgentPlatform は接続先が Agent Platform かを返す。fork でないときは常に false。
func onAgentPlatform(cmd *cobra.Command) bool {
	if !gmn.Enabled() {
		return false
	}
	s, err := backend.Resolve(cmd)
	return err == nil && s.IsVertex()
}

// checkGCSInput は gs:// の入力の MIME type を決める。拡張子で決まらなければ --mime-type を求める。
// 中身は読まないので、URI に載せられない種類（テキスト）は URI の種類の判定だけで止める。
func checkGCSInput(arg, mimeOverride, ref string, policy mediaPolicy) (string, error) {
	mimeType := detectMIME(arg, mimeOverride)
	if mimeType == "" {
		return "", usageError(ref+": cannot determine the MIME type from the gs:// URI",
			"Pass --mime-type explicitly (for example --mime-type audio/mp3)")
	}
	if err := checkRemoteMIME(mimeType, ref, policy); err != nil {
		return "", err
	}
	return mimeType, nil
}

// overflowHint は inline の上限を超えたローカルファイルの直し方。Agent Platform には Files API が無い。
func overflowHint(path string, agentPlatform bool) string {
	if agentPlatform {
		return gcsinput.UploadHint(path)
	}
	return fmt.Sprintf("Run \"gemini-api files upload %s\" and pass the returned files/<id> instead", path)
}
