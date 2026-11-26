package contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gmn の gs:// 入力の契約。Agent Platform には Files API が無いので、
// 大きなファイルは Cloud Storage の URI を file data として渡す。

// gcsVertexArgs は Agent Platform の接続先を stub に向けて組み立てる。--access-token で ADC の読み出しを避ける。
func gcsVertexArgs(stub *interactionStub, command string, extra ...string) []string {
	args := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--server-url", stub.URL, "--access-token", "tok"}
	args = append(args, command)
	return append(args, extra...)
}

// gcsOKResponse は analyze・transcribe が本文を読める最小の応答。
const gcsOKResponse = `{"id":"i","object":"interaction","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"ok"}]}]}`

// uriBlock は入力の content のうち uri を持つ 1 件を返す。無ければ nil。
func uriBlock(parts []map[string]any) map[string]any {
	for _, p := range parts {
		if _, ok := p["uri"]; ok {
			return p
		}
	}
	return nil
}

func TestPlusGCSはAgentPlatformでgs_URIをローカル探索せずURIで送る(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	// 同名のローカルファイルは置かない。探しに行けば file not found で落ちる。
	result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "analyze", "--input", "gs://bucket/path.mp4", "q")...)
	if result.err != nil {
		t.Fatalf("analyze failed: %v\n%s", result.err, result.stderr)
	}
	block := uriBlock(stub.inputParts(t))
	if block == nil {
		t.Fatalf("gs:// の入力が uri として送られていない: %v", stub.requestBody(t))
	}
	if block["uri"] != "gs://bucket/path.mp4" {
		t.Errorf("uri = %v, want gs://bucket/path.mp4", block["uri"])
	}
	if _, ok := block["data"]; ok {
		t.Errorf("gs:// の入力に data（inline の中身）が入っている: %v", block)
	}
}

func TestPlusGCSのMIMEは拡張子から決まり_mime_typeで上書きできる(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"拡張子から", []string{"gs://bucket/a.mp4"}, "video/mp4"},
		{"--mime-type で上書き", []string{"gs://bucket/a.mp4", "--mime-type", "video/webm"}, "video/webm"},
		{"拡張子が未知なら --mime-type を使う", []string{"gs://bucket/a.bin", "--mime-type", "image/png"}, "image/png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newInteractionStub(t, gcsOKResponse)
			args := append([]string{"--input"}, tc.args...)
			result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "analyze", append(args, "q")...)...)
			if result.err != nil {
				t.Fatalf("analyze failed: %v\n%s", result.err, result.stderr)
			}
			block := uriBlock(stub.inputParts(t))
			if block == nil || block["mime_type"] != tc.want {
				t.Errorf("block = %v, want mime_type %s", block, tc.want)
			}
		})
	}
}

func TestPlusGCSのMIMEが決まらないならmime_typeの指定を示して止まる(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "analyze", "--input", "gs://bucket/a.xyz", "q")...)
	if exitCodeOf(result.err) != 2 {
		t.Fatalf("exit = %d, want 2\n%s", exitCodeOf(result.err), result.stderr)
	}
	if !strings.Contains(result.stderr, "--mime-type") {
		t.Errorf("stderr に --mime-type の案内が無い:\n%s", result.stderr)
	}
	if stub.requests.Load() != 0 {
		t.Errorf("止めるべきなのに %d 回送信した", stub.requests.Load())
	}
}

func TestPlusGCSはtranscribeでもURIで送る(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "transcribe", "--input", "gs://bucket/talk.mp3", "--out", filepath.Join(t.TempDir(), "out.txt"))...)
	if result.err != nil {
		t.Fatalf("transcribe failed: %v\n%s", result.err, result.stderr)
	}
	block := uriBlock(stub.inputParts(t))
	if block == nil || block["uri"] != "gs://bucket/talk.mp3" || block["mime_type"] != "audio/mp3" {
		t.Errorf("block = %v, want uri gs://bucket/talk.mp3 with audio/mp3", block)
	}
}

func TestPlusGCSは上限を超えるローカルファイルをgs_に置く方法を示して止まる(t *testing.T) {
	// sparse file なので容量は食わない。base64 にすると約 21 MB になり、inline の上限（約 19 MB）を超える。
	dir := t.TempDir()
	big := filepath.Join(dir, "big.mp4")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(16 << 20); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	stub := newInteractionStub(t, gcsOKResponse)
	result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "analyze", "--input", big, "q")...)
	if exitCodeOf(result.err) != 2 {
		t.Fatalf("exit = %d, want 2\n%s", exitCodeOf(result.err), result.stderr)
	}
	for _, want := range []string{"gs://", "gcloud storage cp"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("stderr に %q が無い:\n%s", want, result.stderr)
		}
	}
	if strings.Contains(result.stderr, "files upload") {
		t.Errorf("Agent Platform で Files API の案内を出している:\n%s", result.stderr)
	}
	if stub.requests.Load() != 0 {
		t.Errorf("止めるべきなのに %d 回送信した", stub.requests.Load())
	}
}

func TestPlusGCSはGeminiAPIでは分かるエラーで止まりローカルとして扱わない(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	args := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--api-key", "k", "--server-url", stub.URL, "analyze", "--input", "gs://bucket/path.mp4", "q"}
	result := runPlus(t, t.TempDir(), nil, args...)
	if exitCodeOf(result.err) != 2 {
		t.Fatalf("exit = %d, want 2\n%s", exitCodeOf(result.err), result.stderr)
	}
	if !strings.Contains(result.stderr, "gs://") || !strings.Contains(result.stderr, "--backend vertex") {
		t.Errorf("Gemini API で gs:// を使えない理由と直し方（--backend vertex）が無い:\n%s", result.stderr)
	}
	if strings.Contains(result.stderr, "file not found") {
		t.Errorf("ローカルファイルとして扱って探している:\n%s", result.stderr)
	}
	if stub.requests.Load() != 0 {
		t.Errorf("止めるべきなのに %d 回送信した", stub.requests.Load())
	}
}
