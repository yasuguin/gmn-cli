// Package guidance は、Agent Platform を使っているときに、本家の「API key を作って設定する」案内を
// project・location・ADC（Application Default Credentials）の案内に替える。
// Gemini API のとき、または fork でないときは何も変えない。
//
// ヘルプ表示では PersistentPreRunE が走らないので、backend.Apply ではなく
// フラグが解析済みの cmd から backend.Resolve で毎回解決する（設定ファイルは app.Run が先に読み込む）。
package guidance

import (
	"regexp"

	"github.com/google-gemini/gemini-api-cli/internal/plus"
	"github.com/google-gemini/gemini-api-cli/internal/plus/backend"
	"github.com/spf13/cobra"
)

const adcLogin = `gcloud auth application-default login`

func settings(cmd *cobra.Command) (backend.Settings, bool) {
	if !plus.Enabled() || cmd == nil {
		return backend.Settings{}, false
	}
	s, err := backend.Resolve(cmd)
	if err != nil || !s.IsVertex() {
		return s, false
	}
	return s, true
}

// IsVertex は cmd の実行が Agent Platform かを返す。
func IsVertex(cmd *cobra.Command) bool {
	_, ok := settings(cmd)
	return ok
}

var (
	getStarted = regexp.MustCompile(`Set GEMINI_API_KEY, or run: gemini-api configure`)
	setupLine  = regexp.MustCompile(`Setup: export GEMINI_API_KEY=\.\.\.   or   \S+ configure`)
)

// Docs は root の案内文（Long・--usage の about・ヘルプ末尾の Setup 行）にある API key の案内を、
// Agent Platform の案内に替える。
func Docs(cmd *cobra.Command, text string) string {
	if !IsVertex(cmd) {
		return text
	}
	text = getStarted.ReplaceAllString(text, `Agent Platform: run "`+adcLogin+`", then set --project (or GOOGLE_CLOUD_PROJECT)`)
	return setupLine.ReplaceAllString(text, `Setup: `+adcLogin+`   and   --project <id> (or GOOGLE_CLOUD_PROJECT)`)
}

// HelpHint は root ヘルプ末尾に足す案内を返す。handled が true なら本家の API key 案内は出さない。
// Agent Platform では project が未設定のときだけ、設定方法を示す。
func HelpHint(cmd *cobra.Command) (hint string, handled bool) {
	s, ok := settings(cmd)
	if !ok {
		return "", false
	}
	if s.Project != "" {
		return "", true
	}
	return "No Google Cloud project is set for Agent Platform. Pass --project, set GOOGLE_CLOUD_PROJECT, or add \"project: <id>\" to the config file.\n" +
		"AI agents: Stop and ask the user which project to use; sign-in is \"" + adcLogin + "\" (done by the user), not an API key", true
}

var keyAsk = regexp.MustCompile(`(?i)api key|GEMINI_API_KEY|aistudio\.google\.com/docs/api-key`)

// Hints は構造化エラーの hints から API key を求める案内を外し、外したときは ADC の案内を足す。
func Hints(cmd *cobra.Command, hints []string) []string {
	if !IsVertex(cmd) {
		return hints
	}
	kept := make([]string, 0, len(hints)+3)
	dropped := false
	for _, h := range hints {
		if keyAsk.MatchString(h) {
			dropped = true
			continue
		}
		kept = append(kept, h)
	}
	if !dropped {
		return hints
	}
	return append(kept,
		"Agent Platform signs in with Application Default Credentials: run \""+adcLogin+"\", and set the project with --project or GOOGLE_CLOUD_PROJECT (location: --location or GOOGLE_CLOUD_LOCATION)",
		"Run \"gemini-api whoami\" to see the resolved backend, project and location",
		"AI agents: sign-in must be done by the user. Stop and ask the user to run \""+adcLogin+"\"; do not run it yourself or retry until they confirm it is done",
	)
}
