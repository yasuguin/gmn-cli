package plus_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google-gemini/gemini-api-cli/internal/plus"
)

func TestReplaceCommandName(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"コマンド名", `Run: gemini-api configure`, `Run: gmn configure`},
		{"引用符内", `"gemini-api whoami"`, `"gmn whoami"`},
		{"文末のピリオド", `use gemini-api.`, `use gmn.`},
		{"設定ファイルの場所", `~/.config/gemini-api/config.yaml`, `~/.config/gmn/config.yaml`},
		{"リポジトリ名は対象外", `gemini-api-cli`, `gemini-api-cli`},
		{"URL は対象外", `https://github.com/google-gemini/gemini-api`, `https://github.com/google-gemini/gemini-api`},
		{"go install のパスは対象外", `./cmd/gemini-api`, `./cmd/gemini-api`},
		{"別の単語の一部は対象外", `xgemini-api`, `xgemini-api`},
		{"環境変数名は対象外", `GEMINI_API_KEY`, `GEMINI_API_KEY`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := plus.ReplaceCommandName(tc.in); got != tc.want {
				t.Errorf("ReplaceCommandName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// 置き換え後の名前は本家のコマンド名を含まないので、出力が再び置き換えられることはない。
func TestForkNameは本家のコマンド名を含まない(t *testing.T) {
	if strings.Contains(plus.Name, "gemini-api") {
		t.Errorf("plus.Name = %q は本家の gemini-api を含む", plus.Name)
	}
}

// 書き込みの区切りが名前の途中でも結果が同じになる。
func TestWriterは分割書き込みでも置き換える(t *testing.T) {
	input := "Run: gemini-api configure\nsee ~/.config/gemini-api/config.yaml and gemini-api-cli, then gemini-api"
	want := plus.ReplaceCommandName(input)
	for size := 1; size <= len(input); size++ {
		var dst bytes.Buffer
		w := plus.NewWriter(&dst)
		for i := 0; i < len(input); i += size {
			end := min(i+size, len(input))
			if _, err := w.Write([]byte(input[i:end])); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
		if got := dst.String(); got != want {
			t.Fatalf("size=%d: got %q, want %q", size, got, want)
		}
	}
}

// 名前に関係ない出力はその場で流れ、ストリーミングを遅らせない。
func TestWriterは無関係な出力を保留しない(t *testing.T) {
	var dst bytes.Buffer
	w := plus.NewWriter(&dst)
	if _, err := w.Write([]byte("hello world")); err != nil {
		t.Fatal(err)
	}
	if got := dst.String(); got != "hello world" {
		t.Errorf("flush 前の出力 = %q, want %q", got, "hello world")
	}
}

func TestUserAgentとVersionは有効化するまで本家のまま(t *testing.T) {
	if plus.Enabled() {
		t.Skip("他のテストが有効化済み")
	}
	if got := plus.UserAgent("google-genai-cli/0.5.0 gl-go/go1"); got != "google-genai-cli/0.5.0 gl-go/go1" {
		t.Errorf("UserAgent = %q", got)
	}
	if got := plus.VersionHeader("gemini-api 0.5.0", "0.5.0"); got != "gemini-api 0.5.0" {
		t.Errorf("VersionHeader = %q", got)
	}
}
