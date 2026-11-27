// Package capability は、コマンドごとに Agent Platform（旧 Vertex AI）でどう動くかを 1 か所の表で持つ。
//
// 表のキーは cobra の CommandPath から root 名を除いたコマンドパス（例 "files upload"）。
// 子コマンドは、表に載っている一番近い親の扱いを継ぐ。
// #11・#12（image・tts を generateContent に切り替える）と #8（モデルの読み替え）は、
// Lookup で自分のコマンドの Kind を引いて分岐する。
package capability

import (
	"strings"

	"github.com/spf13/cobra"
)

// Kind は Agent Platform でのコマンドの扱い。
type Kind string

const (
	// Local は API を呼ばないコマンド（設定・案内）。接続先に依らず動く。
	Local Kind = "local"
	// Interactions は本家と同じ Interactions API のまま動く。
	Interactions Kind = "interactions"
	// GenerateContent は Agent Platform では generateContent に切り替えて動かす（実装は #11・#12）。
	GenerateContent Kind = "generate-content"
	// GCS はローカルファイルを受け付けず、gs:// の URI を使う。
	GCS Kind = "gcs"
	// Unsupported は Gemini API の機能で、Agent Platform では使えない。
	Unsupported Kind = "unsupported"
	// Unverified は Agent Platform での動作が未確認。止めず、本家の挙動のまま送る。
	Unverified Kind = "unverified"
)

// Entry は表の 1 行。Note は止めるときの理由に使う。
type Entry struct {
	Kind Kind
	Note string
}

const geminiOnly = "is a Gemini API feature (managed agent infrastructure) with no Agent Platform counterpart"

// table が能力表の本体。新しいコマンドを足したときは網羅テスト（capability_test.go）が気付かせる。
var table = map[string]Entry{
	// API を呼ばない
	"auth":       {Kind: Local},
	"completion": {Kind: Local},
	"configure":  {Kind: Local},
	"explore":    {Kind: Local},
	"help":       {Kind: Local},
	"version":    {Kind: Local},
	"whoami":     {Kind: Local},

	// Interactions のまま
	"agent":      {Kind: Interactions},
	"analyze":    {Kind: Interactions},
	"generate":   {Kind: Interactions},
	"transcribe": {Kind: Interactions},

	// generateContent に切り替える（今は止めない）
	"image": {Kind: GenerateContent},
	"tts":   {Kind: GenerateContent},

	// 未確認（smoke で確定するまで止めない）
	"music":  {Kind: Unverified},
	"video":  {Kind: Unverified},
	"models": {Kind: Unverified},

	// gs:// を使う
	"files upload": {Kind: GCS, Note: "local files cannot be uploaded (Agent Platform has no Files API)"},

	// Gemini API だけ
	"files":        {Kind: Unsupported, Note: "the Files API " + geminiOnly},
	"credentials":  {Kind: Unsupported, Note: "credentials " + geminiOnly},
	"environments": {Kind: Unsupported, Note: "environments " + geminiOnly},
	"triggers":     {Kind: Unsupported, Note: "triggers " + geminiOnly},
	"voices":       {Kind: Unsupported, Note: "custom voices " + geminiOnly},
	"webhooks":     {Kind: Unsupported, Note: "webhooks " + geminiOnly},
}

// Lookup は path の扱いと、その扱いを決めた表のキーを返す。
// 表に無いコマンドは、一番近い親の扱いを継ぐ。親も無ければ Unverified。
// 本家の追従で増えたコマンドを、確かめないまま止めないため。
func Lookup(path string) (Entry, string) {
	for p := path; p != ""; {
		if e, ok := table[p]; ok {
			return e, p
		}
		i := strings.LastIndex(p, " ")
		if i < 0 {
			break
		}
		p = p[:i]
	}
	return Entry{Kind: Unverified}, ""
}

// PathOf は cmd の表のキー（root 名を除いた CommandPath）を返す。
func PathOf(cmd *cobra.Command) string {
	_, rest, _ := strings.Cut(cmd.CommandPath(), " ")
	return rest
}
