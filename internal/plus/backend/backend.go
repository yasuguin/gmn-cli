// Package backend は gmn の接続先（Gemini API / Agent Platform）の解決を担う。
//
// Agent Platform（旧 Vertex AI）の URL は本家の
// "{server-url}/{api-version}/interactions" の形に収まるので、
// --server-url に "https://{host}/v1beta1/projects/{project}/locations"、
// --api-version に location を入れて本家の仕組み（SDK と custom/rawhttp の両方）に載せる。
// 本家のコードは接続先を知らずに済み、差分は Apply を呼ぶ 1 行で済む。
package backend

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/google-gemini/gemini-api-cli/internal/config"
	"github.com/google-gemini/gemini-api-cli/internal/flagutil"
	"github.com/google-gemini/gemini-api-cli/internal/plus"
	"github.com/spf13/cobra"
)

const (
	// Gemini は既定の接続先（Gemini Developer API）。
	Gemini = "gemini"
	// Vertex は Agent Platform（旧 Vertex AI）。CLI の値は公式 SDK に合わせて vertex。
	Vertex = "vertex"

	// DefaultLocation は location を指定しなかったときの値。
	DefaultLocation = "global"

	// APIVersion は Agent Platform の Interactions API の版。
	APIVersion = "v1beta1"
)

// Settings は解決済みの接続先。Source は "flag"・"env"・"config"・"default"・"unset"。
type Settings struct {
	Backend, BackendSource   string
	Project, ProjectSource   string
	Location, LocationSource string
}

// IsVertex は Agent Platform が選ばれているかを返す。
func (s Settings) IsVertex() bool { return s.Backend == Vertex }

// Host は Agent Platform のホスト名。global だけ location を付けない。
func (s Settings) Host() string {
	if s.Location == DefaultLocation {
		return "aiplatform.googleapis.com"
	}
	return s.Location + "-aiplatform.googleapis.com"
}

// ServerURL は本家の server-url に渡す値。末尾の "/locations" の後ろに api-version（= location）が続く。
func (s Settings) ServerURL() string {
	return "https://" + s.Host() + "/" + APIVersion + "/projects/" + s.Project + "/locations"
}

var current atomic.Pointer[Settings]

// Current は最後に Apply した設定を返す。Apply 前は nil。
func Current() *Settings { return current.Load() }

// RegisterFlags は --backend・--project・--location を足す。fork でないときは何もしない。
func RegisterFlags(root *cobra.Command) {
	if !plus.Enabled() {
		return
	}
	pf := root.PersistentFlags()
	pf.String("backend", "", "Backend to call: gemini (Gemini API, default) or vertex (Agent Platform). env: GOOGLE_GENAI_USE_VERTEXAI=true, config: backend")
	pf.String("project", "", "Google Cloud project for --backend vertex (env: GOOGLE_CLOUD_PROJECT, config: project)")
	pf.String("location", "", "Agent Platform location for --backend vertex (env: GOOGLE_CLOUD_LOCATION, config: location; default: global)")
	for _, name := range []string{"backend", "project", "location"} {
		_ = pf.SetAnnotation(name, "speakeasy:group", []string{"API Parameters"})
	}
}

// Resolve は接続先を フラグ > 環境変数 > 設定ファイル > 既定 の順で解決する。
// Gemini API か Agent Platform かは、値が無い限り自動では判定しない。
func Resolve(cmd *cobra.Command) (Settings, error) {
	var s Settings
	if v, changed := flagutil.GetStringFlag(cmd, "backend"); changed && v != "" {
		s.Backend, s.BackendSource = strings.ToLower(strings.TrimSpace(v)), "flag"
	} else if v := parseUseVertexEnv(os.Getenv("GOOGLE_GENAI_USE_VERTEXAI")); v != "" {
		s.Backend, s.BackendSource = v, "env"
	} else if v := config.GetConfigValue("backend"); v != "" {
		s.Backend, s.BackendSource = strings.ToLower(strings.TrimSpace(v)), "config"
	} else {
		s.Backend, s.BackendSource = Gemini, "default"
	}
	if s.Backend != Gemini && s.Backend != Vertex {
		return s, flagutil.WithCLIValidation(fmt.Errorf("invalid backend %q (from %s): use %q or %q", s.Backend, s.BackendSource, Gemini, Vertex))
	}

	s.Project, s.ProjectSource = pick(cmd, "project", "GOOGLE_CLOUD_PROJECT", "")
	s.Location, s.LocationSource = pick(cmd, "location", "GOOGLE_CLOUD_LOCATION", DefaultLocation)
	return s, nil
}

func pick(cmd *cobra.Command, flag, env, def string) (string, string) {
	if v, changed := flagutil.GetStringFlag(cmd, flag); changed && v != "" {
		return strings.TrimSpace(v), "flag"
	}
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v, "env"
	}
	if v := strings.TrimSpace(config.GetConfigValue(flag)); v != "" {
		return v, "config"
	}
	if def != "" {
		return def, "default"
	}
	return "", "unset"
}

// parseUseVertexEnv は GOOGLE_GENAI_USE_VERTEXAI を backend 値にする。
// true/1 は vertex、false/0 は gemini（設定ファイルの vertex を環境変数で戻せる）、それ以外は未指定。
func parseUseVertexEnv(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1":
		return Vertex
	case "false", "0":
		return Gemini
	}
	return ""
}

// Apply は接続先を解決し、Agent Platform なら本家の仕組みが読むフラグ値へ落とす。
// 本家の PersistentPreRunE から呼ぶ。fork でないとき・Gemini API のときは何も変えない。
func Apply(cmd *cobra.Command) error {
	if !plus.Enabled() {
		return nil
	}
	s, err := Resolve(cmd)
	if err != nil {
		return err
	}
	current.Store(&s)
	adcPending.Store(false)
	projectMissing.Store(false)
	plus.SetAPIKeyDisabled(s.IsVertex())
	if !s.IsVertex() {
		return nil
	}
	// project の要求は最初のリクエストまで遅らせる。whoami・configure・version・--schema のように
	// リクエストを送らないコマンドは project が無くても動かす（WrapHTTPClient が送る直前に止める）。
	projectMissing.Store(s.Project == "")

	// --server-url を明示したときはテストのスタブなどへの送り先を優先し、URL は組み立てない。
	if _, changed := flagutil.GetStringFlag(cmd, "server-url"); !changed {
		if err := cmd.Flags().Set("server-url", s.ServerURL()); err != nil {
			return err
		}
		if err := cmd.Flags().Set("api-version", s.Location); err != nil {
			return err
		}
	}
	// access-token の明示（フラグ・環境変数・keyring・設定）が無ければ ADC に任せる。
	// 読み出しは最初のリクエストまで遅らせ、whoami や configure のようにリクエストを送らない
	// コマンドが ADC の有無で失敗しないようにする（トークンはリクエストごとに WrapHTTPClient が付ける）。
	_, tokenSource := config.ResolveRequestSecurityCredential(cmd, "access-token")
	adcPending.Store(tokenSource == "unset")
	// 課金・クォータの project は、明示（フラグ・環境変数・設定）が無ければ project に揃える。
	if _, changed := flagutil.GetStringFlag(cmd, "user-project"); !changed && config.GetString("user-project") == "" && s.Project != "" {
		if err := cmd.Flags().Set("user-project", s.Project); err != nil {
			return err
		}
	}
	return nil
}

type missingProjectError struct{}

func (missingProjectError) Error() string {
	return "--backend vertex needs a Google Cloud project: pass --project, set GOOGLE_CLOUD_PROJECT, or add \"project: <id>\" to " +
		config.GetConfigPath() + " (run `" + plus.Name + " whoami` to see the resolved backend)"
}

// WhoamiInfo は whoami 用に backend・project・location と取得元を返す。
func WhoamiInfo(cmd *cobra.Command) (map[string]any, []Row) {
	s, err := Resolve(cmd)
	if err != nil {
		s = Settings{Backend: Gemini, BackendSource: "default", Location: DefaultLocation, LocationSource: "default", ProjectSource: "unset"}
	}
	rows := []Row{
		{"backend", s.BackendSource, s.Backend},
		{"project", s.ProjectSource, s.Project},
		{"location", s.LocationSource, s.Location},
	}
	if s.IsVertex() {
		rows = append(rows, authRow(cmd))
	}
	info := map[string]any{}
	for _, r := range rows {
		info[r.Name] = map[string]any{"source": r.Source, "value": r.Value}
	}
	return info, rows
}

// authRow は認証の出どころ。明示の access-token（トークン本体は出さない）か ADC。
func authRow(cmd *cobra.Command) Row {
	if _, source := config.ResolveRequestSecurityCredential(cmd, "access-token"); source != "unset" {
		return Row{"auth", source, "access-token"}
	}
	return Row{"auth", authADC, "ADC (read when a request is sent)"}
}

// AnnotateWhoami は whoami の機械可読出力に backend を足す。fork でないときは何もしない。
func AnnotateWhoami(cmd *cobra.Command, info map[string]any) {
	if !plus.Enabled() {
		return
	}
	info["backend"], _ = WhoamiInfo(cmd)
}

// Row は whoami の 1 行。
type Row struct{ Name, Source, Value string }

// PrintWhoami は whoami の人間向け出力に Backend 節を出す。fork でないときは何も出さない。
func PrintWhoami(out io.Writer, cmd *cobra.Command) {
	if !plus.Enabled() {
		return
	}
	_, rows := WhoamiInfo(cmd)
	fmt.Fprintln(out, "Backend:")
	for _, r := range rows {
		name := "--" + r.Name
		if r.Name == "auth" {
			name = "auth" // フラグではない
		}
		fmt.Fprintf(out, "  %-27s [%-7s] %s\n", name, r.Source, r.Value)
	}
	fmt.Fprintln(out)
}
