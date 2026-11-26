// Package modelmap は、モデルごとに Agent Platform でどう扱うかを 1 か所の表で持つ（#7 の能力表はコマンド単位）。
//
// 表に無いモデルは Unverified とし、止めずにそのまま送ってサーバーの判定に任せる。
// #11・#12（image・tts を generateContent に切り替える）は、Lookup で自分のモデルの Status を引いて分岐する。
package modelmap

import (
	"fmt"
	"strings"

	"github.com/google-gemini/gemini-api-cli/internal/clierrors"
	"github.com/google-gemini/gemini-api-cli/internal/plus"
	"github.com/google-gemini/gemini-api-cli/internal/plus/backend"
	"github.com/google-gemini/gemini-api-cli/internal/plus/capability"
	"github.com/spf13/cobra"
)

// Status は Agent Platform でのモデルの扱い。
type Status string

const (
	// Verified は Interactions API でそのまま通ると実測したモデル。
	Verified Status = "verified"
	// Renamed は Agent Platform での名前が違うモデル。Entry.Name へ読み替えて送る。
	Renamed Status = "renamed"
	// Unsupported は Agent Platform に無いモデル（Interactions API が拒否する）。Entry.Alternatives が代わりの候補。
	Unsupported Status = "unsupported"
	// GenerateContent は Interactions API では通らず、generateContent で呼ぶモデル（image・tts）。
	// 止めずにそのまま送る。
	GenerateContent Status = "generate-content"
	// Unverified は Agent Platform での動作が未確認のモデル。止めずにそのまま送る。
	Unverified Status = "unverified"
)

// Entry は表の 1 行。
type Entry struct {
	Status Status
	// Name は Renamed のときの Agent Platform での名前。
	Name string
	// Alternatives は Unsupported のときの代わりの候補。先頭が既定の代わりに使われる。
	Alternatives []string
}

const transcribeFallback = "gemini-3.8-flash"

// table が表の本体。キーは Gemini API と共通のモデル名。
// 実測は 2026-11-26（global）。Interactions API は generate --model、generateContent は image・smoke の --probe-models で確かめた。
var table = map[string]Entry{
	"gemini-4-hydrogen": {Status: Verified},
	"gemini-3.8-flash":  {Status: Verified},
	"gemini-3.7-flash":  {Status: Verified},
	"gemini-3.5-flash":  {Status: Verified},
	"gemini-4-argon":    {Status: Verified},
	"gemini-4-carbon":   {Status: Verified},
	"gemini-4-neon":     {Status: Verified},

	"gemini-3.5-transcribe": {Status: Unsupported, Alternatives: []string{transcribeFallback}},

	// Interactions API が 400（not supported）で拒否した。代わりは Interactions で通ったモデルから近い世代・用途のものにする:
	// flash 系と gemma は軽量の gemini-3.5-flash、pro 系は上位の gemini-4-argon。
	"gemini-2.5-flash":    {Status: Unsupported, Alternatives: []string{"gemini-3.5-flash"}},
	"gemini-flash-latest": {Status: Unsupported, Alternatives: []string{"gemini-3.5-flash"}},
	"gemma-4-31b-it":      {Status: Unsupported, Alternatives: []string{"gemini-3.5-flash"}},
	"gemini-2.5-pro":      {Status: Unsupported, Alternatives: []string{"gemini-4-argon"}},
	"gemini-pro-latest":   {Status: Unsupported, Alternatives: []string{"gemini-4-argon"}},

	// image は generateContent で通った（Interactions API は拒否）。nano-banana-pro-preview は publisher model が無く 404。
	"gemini-nano-banana-2.1":  {Status: GenerateContent},
	"gemini-3.1-flash-image":  {Status: GenerateContent},
	"gemini-3-pro-image":      {Status: GenerateContent},
	"gemini-2.5-flash-image":  {Status: GenerateContent},
	"nano-banana-pro-preview": {Status: Unsupported, Alternatives: []string{"gemini-3-pro-image"}},

	// tts は generateContent で通った（実測 2026-11-26）。
	"gemini-3.8-flash-tts":         {Status: GenerateContent},
	"gemini-3.8-flash-lite-tts":    {Status: GenerateContent},
	"gemini-3.1-flash-tts-preview": {Status: GenerateContent},
	"gemini-2.5-flash-preview-tts": {Status: GenerateContent},
}

// Lookup は共通名のモデルの扱いを返す。表に無ければ Unverified。
func Lookup(id string) Entry {
	if e, ok := table[strings.TrimPrefix(id, "models/")]; ok {
		return e
	}
	return Entry{Status: Unverified}
}

// resolve は 1 件のエントリに従って送るモデル名を決める。
// 表を引かない形にしてあるのは、Renamed の読み替えを実在の名前なしに単体テストするため。
// explicit が true で Unsupported のときは勝手に替えず errUnsupported を返す。false なら先頭の候補で替える。
func resolve(id string, e Entry, explicit bool) (use string, replaced bool, err error) {
	switch e.Status {
	case Renamed:
		return e.Name, e.Name != id, nil
	case Unsupported:
		if explicit || len(e.Alternatives) == 0 {
			return id, false, &unsupportedError{model: id, alternatives: e.Alternatives}
		}
		return e.Alternatives[0], true, nil
	}
	return id, false, nil
}

// onAgentPlatform は fork で、接続先が Agent Platform のときに true。Apply 済みでも前でも使えるよう Resolve で引く。
func onAgentPlatform(cmd *cobra.Command) bool {
	if !plus.Enabled() {
		return false
	}
	s, err := backend.Resolve(cmd)
	return err == nil && s.IsVertex()
}

// ResolveDefault は --model 未指定で既定のモデル def を使うときの送り先を返す。
// 既定が Agent Platform に無ければ代わりのモデルにし、その旨を stderr に 1 行出す（stdout は汚さない）。
// Gemini API のときは何もしない。
func ResolveDefault(cmd *cobra.Command, def string) string {
	if !onAgentPlatform(cmd) {
		return def
	}
	use, replaced, err := resolve(def, Lookup(def), false)
	if err != nil {
		return def
	}
	if replaced {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s は Agent Platform に無いので %s を使う\n", def, use)
	}
	return use
}

// Guard は --model で明示されたモデルが Agent Platform に無いとき、通信する前に候補つきのエラーで止める。
// 本家の PersistentPreRunE の末尾（backend.Apply の前）から呼ぶ。
// --model が models get の対象指定でもあるため、モデルを選ぶコマンド（Interactions・GenerateContent）だけを対象にする。
func Guard(cmd *cobra.Command) error {
	if !onAgentPlatform(cmd) || cmd.Flags().Lookup("model") == nil || !cmd.Flags().Changed("model") {
		return nil
	}
	if k := kindOf(cmd); k != capability.Interactions && k != capability.GenerateContent {
		return nil
	}
	id, _ := cmd.Flags().GetString("model")
	id = strings.TrimPrefix(strings.TrimSpace(id), "models/")
	use, replaced, err := resolve(id, Lookup(id), true)
	if err != nil {
		return clierrors.WithExitCode(err, clierrors.ExitUsage)
	}
	if replaced {
		return cmd.Flags().Set("model", use)
	}
	return nil
}

func kindOf(cmd *cobra.Command) capability.Kind {
	e, _ := capability.Lookup(capability.PathOf(cmd))
	return e.Kind
}

// unsupportedError は CLI_VALIDATION の構造化エラー。呼び方の誤りなので終了コードは 2。
type unsupportedError struct {
	model        string
	alternatives []string
}

func (e *unsupportedError) Error() string {
	return fmt.Sprintf("model %q is not available on Agent Platform", e.model)
}

func (e *unsupportedError) CLIReason() string { return "CLI_VALIDATION" }

func (e *unsupportedError) CLIHints() []string {
	hints := []string{}
	if len(e.alternatives) > 0 {
		hints = append(hints, "Use one of: "+strings.Join(e.alternatives, ", ")+" (pass it with --model)")
	}
	return append(hints,
		"Run \"gemini-api models\" to see which models work on Agent Platform",
		"To use this model, switch to the Gemini API: pass --backend gemini (or set GOOGLE_GENAI_USE_VERTEXAI=false)",
	)
}

func (e *unsupportedError) CLIOmitTypeHints() bool { return true }
