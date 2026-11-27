package contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// モデル名の読み替え（#8）の契約。Agent Platform で通らないと分かっている既定モデルだけを代わりに替え、
// 明示された未確認のモデルは止めずにそのまま送る。Gemini API のときは本家のまま。

func modelOf(t *testing.T, stub *interactionStub) any {
	t.Helper()
	return stub.requestBody(t)["model"]
}

func TestGmnモデル_AgentPlatformのtranscribe既定は代わりのモデルで送りstderrに英語で出す(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	// 既定の出力形式（-o 無し）のときだけ stderr に出る。
	args := withoutOutputFormat(gcsVertexArgs(stub, "transcribe", "--input", "gs://bucket/talk.mp3", "--out", filepath.Join(t.TempDir(), "out.txt")))
	result := runGmn(t, t.TempDir(), nil, args...)
	if result.err != nil {
		t.Fatalf("transcribe failed: %v\n%s", result.err, result.stderr)
	}
	if got := modelOf(t, stub); got != "gemini-3.8-flash" {
		t.Errorf("model = %v, want gemini-3.8-flash", got)
	}
	want := "Using gemini-3.8-flash: gemini-3.5-transcribe is not available on Agent Platform"
	if !strings.Contains(result.stderr, want) {
		t.Errorf("stderr に %q が無い:\n%s", want, result.stderr)
	}
	if strings.Contains(result.stdout, "gemini-3.5-transcribe") {
		t.Errorf("stdout に読み替えの案内が混ざっている:\n%s", result.stdout)
	}
}

func TestGmnモデル_AgentPlatformの既定の読み替えはjsonでは出力に出さない(t *testing.T) {
	for _, flag := range []string{"-o", "--agent-mode"} {
		stub := newInteractionStub(t, gcsOKResponse)
		args := withoutOutputFormat(gcsVertexArgs(stub, "transcribe", "--input", "gs://bucket/talk.mp3", "--out", filepath.Join(t.TempDir(), "out.txt")))
		if flag == "-o" {
			args = append([]string{"-o", "json"}, args...)
		} else {
			args = append([]string{flag}, args...)
		}
		result := runGmn(t, t.TempDir(), nil, args...)
		if result.err != nil {
			t.Fatalf("%s: transcribe failed: %v\n%s", flag, result.err, result.stderr)
		}
		if strings.Contains(result.stderr, "gemini-3.5-transcribe") {
			t.Errorf("%s: stderr に案内が出ている:\n%s", flag, result.stderr)
		}
		if got := modelOf(t, stub); got != "gemini-3.8-flash" {
			t.Errorf("%s: model = %v, want gemini-3.8-flash", flag, got)
		}
	}
}

// withoutOutputFormat は引数列から "-o json" を取り除く。
func withoutOutputFormat(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func TestGmnモデル_AgentPlatformでunsupportedのモデルを明示すると通信せず候補を示して終了コード2(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runGmn(t, t.TempDir(), nil, gcsVertexArgs(stub, "transcribe", "--model", "gemini-3.5-transcribe", "--input", "gs://bucket/talk.mp3", "--out", filepath.Join(t.TempDir(), "out.txt"))...)
	if exitCodeOf(result.err) != 2 {
		t.Fatalf("exit = %d, want 2\n%s", exitCodeOf(result.err), result.stderr)
	}
	for _, want := range []string{"gemini-3.5-transcribe", "gemini-3.8-flash"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("エラーに %q が無い:\n%s", want, result.stderr)
		}
	}
	if stub.requests.Load() != 0 {
		t.Errorf("止めるべきなのに %d 回送信した", stub.requests.Load())
	}
}

func TestGmnモデル_AgentPlatformで未確認のモデルはそのまま送る(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runGmn(t, t.TempDir(), nil, gcsVertexArgs(stub, "transcribe", "--model", "some-unlisted-model", "--input", "gs://bucket/talk.mp3", "--out", filepath.Join(t.TempDir(), "out.txt"))...)
	if result.err != nil {
		t.Fatalf("transcribe failed: %v\n%s", result.err, result.stderr)
	}
	if got := modelOf(t, stub); got != "some-unlisted-model" {
		t.Errorf("model = %v, want some-unlisted-model", got)
	}
}

func TestGmnモデル_AgentPlatformのgenerate既定はそのまま送る(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runGmn(t, t.TempDir(), nil, gcsVertexArgs(stub, "generate", "hi")...)
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if got := modelOf(t, stub); got != "gemini-4-hydrogen" {
		t.Errorf("model = %v, want gemini-4-hydrogen", got)
	}
	if strings.Contains(result.stderr, "is not available on Agent Platform") {
		t.Errorf("読み替えていないのに案内が出た:\n%s", result.stderr)
	}
}

func TestGmnモデル_generateで明示したunsupportedのモデルも通信せず止まる(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runGmn(t, t.TempDir(), nil, gcsVertexArgs(stub, "generate", "--model", "gemini-3.5-transcribe", "hi")...)
	if exitCodeOf(result.err) != 2 || stub.requests.Load() != 0 {
		t.Errorf("exit = %d, requests = %d, want 2 と 0\n%s", exitCodeOf(result.err), stub.requests.Load(), result.stderr)
	}
}

func TestGmnモデル_GeminiAPIではtranscribeの既定も明示も読み替えない(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "a.mp3")
	if err := os.WriteFile(audio, []byte("ID3"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		extra []string
	}{
		{"既定", nil},
		{"明示", []string{"--model", "gemini-3.5-transcribe"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := newInteractionStub(t, gcsOKResponse)
			args := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "json", "--api-key", "k", "--server-url", stub.URL,
				"transcribe", "--input", audio, "--out", filepath.Join(t.TempDir(), "out.txt")}
			result := runGmn(t, t.TempDir(), nil, append(args, tc.extra...)...)
			if result.err != nil {
				t.Fatalf("transcribe failed: %v\n%s", result.err, result.stderr)
			}
			if got := modelOf(t, stub); got != "gemini-3.5-transcribe" {
				t.Errorf("model = %v, want gemini-3.5-transcribe", got)
			}
			if strings.Contains(result.stderr, "Agent Platform") {
				t.Errorf("Gemini API なのに Agent Platform の案内が出た:\n%s", result.stderr)
			}
		})
	}
}

func TestGmnモデル_modelsのカタログはAgentPlatform選択時だけ状態を出す(t *testing.T) {
	vertex := runGmn(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--backend", "vertex", "--project", "my-proj", "models")
	if vertex.err != nil {
		t.Fatalf("models failed: %v\n%s", vertex.err, vertex.stderr)
	}
	for _, want := range []string{"gemini-3.5-transcribe", "Agent Platform", "gemini-3.8-flash"} {
		if !strings.Contains(vertex.stdout, want) {
			t.Errorf("Agent Platform のカタログに %q が無い:\n%s", want, vertex.stdout)
		}
	}
	gemini := runGmn(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "models")
	if gemini.err != nil {
		t.Fatalf("models failed: %v\n%s", gemini.err, gemini.stderr)
	}
	if strings.Contains(gemini.stdout, "Agent Platform") {
		t.Errorf("Gemini API のカタログに Agent Platform の状態が出ている")
	}

	machine := runGmn(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "-o", "json", "--backend", "vertex", "--project", "my-proj", "models")
	if machine.err != nil || !strings.Contains(machine.stdout, "agent_platform") {
		t.Errorf("機械可読のカタログに agent_platform が無い: %v\n%s", machine.err, machine.stdout)
	}
}

func TestGmnモデル_カタログの機械出力のエラーを捨てず終了コードに反映する(t *testing.T) {
	result := runGmn(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--backend", "vertex", "--project", "p",
		"-o", "json", "--jq", `error("boom")`, "models")
	if result.err == nil {
		t.Fatalf("--jq の評価エラーが捨てられて成功した\n%s", result.stdout)
	}
}
