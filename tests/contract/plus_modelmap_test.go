package contract_test

import (
	"encoding/json"
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

func TestPlusモデル_AgentPlatformのtranscribe既定は代わりのモデルで送りstderrにだけ出す(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "transcribe", "--input", "gs://bucket/talk.mp3", "--out", filepath.Join(t.TempDir(), "out.txt"))...)
	if result.err != nil {
		t.Fatalf("transcribe failed: %v\n%s", result.err, result.stderr)
	}
	if got := modelOf(t, stub); got != "gemini-3.8-flash" {
		t.Errorf("model = %v, want gemini-3.8-flash", got)
	}
	if !strings.Contains(result.stderr, "gemini-3.5-transcribe") || !strings.Contains(result.stderr, "gemini-3.8-flash") {
		t.Errorf("stderr に読み替えの案内が無い:\n%s", result.stderr)
	}
	if strings.Contains(result.stdout, "gemini-3.5-transcribe") {
		t.Errorf("stdout に読み替えの案内が混ざっている:\n%s", result.stdout)
	}
	var out any
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Errorf("-o json の stdout が JSON でなくなった: %v\n%s", err, result.stdout)
	}
}

func TestPlusモデル_AgentPlatformでunsupportedのモデルを明示すると通信せず候補を示して終了コード2(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "transcribe", "--model", "gemini-3.5-transcribe", "--input", "gs://bucket/talk.mp3", "--out", filepath.Join(t.TempDir(), "out.txt"))...)
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

func TestPlusモデル_AgentPlatformで未確認のモデルはそのまま送る(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "transcribe", "--model", "gemini-2.5-flash", "--input", "gs://bucket/talk.mp3", "--out", filepath.Join(t.TempDir(), "out.txt"))...)
	if result.err != nil {
		t.Fatalf("transcribe failed: %v\n%s", result.err, result.stderr)
	}
	if got := modelOf(t, stub); got != "gemini-2.5-flash" {
		t.Errorf("model = %v, want gemini-2.5-flash", got)
	}
}

func TestPlusモデル_AgentPlatformのgenerate既定はそのまま送る(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "generate", "hi")...)
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if got := modelOf(t, stub); got != "gemini-4-hydrogen" {
		t.Errorf("model = %v, want gemini-4-hydrogen", got)
	}
	if strings.Contains(result.stderr, "Agent Platform に無い") {
		t.Errorf("読み替えていないのに案内が出た:\n%s", result.stderr)
	}
}

func TestPlusモデル_generateで明示したunsupportedのモデルも通信せず止まる(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	result := runPlus(t, t.TempDir(), nil, gcsVertexArgs(stub, "generate", "--model", "gemini-3.5-transcribe", "hi")...)
	if exitCodeOf(result.err) != 2 || stub.requests.Load() != 0 {
		t.Errorf("exit = %d, requests = %d, want 2 と 0\n%s", exitCodeOf(result.err), stub.requests.Load(), result.stderr)
	}
}

func TestPlusモデル_GeminiAPIではtranscribeの既定も明示も読み替えない(t *testing.T) {
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
			result := runPlus(t, t.TempDir(), nil, append(args, tc.extra...)...)
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

func TestPlusモデル_modelsのカタログはAgentPlatform選択時だけ状態を出す(t *testing.T) {
	vertex := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--backend", "vertex", "--project", "my-proj", "models")
	if vertex.err != nil {
		t.Fatalf("models failed: %v\n%s", vertex.err, vertex.stderr)
	}
	for _, want := range []string{"gemini-3.5-transcribe", "Agent Platform", "gemini-3.8-flash"} {
		if !strings.Contains(vertex.stdout, want) {
			t.Errorf("Agent Platform のカタログに %q が無い:\n%s", want, vertex.stdout)
		}
	}
	gemini := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "models")
	if gemini.err != nil {
		t.Fatalf("models failed: %v\n%s", gemini.err, gemini.stderr)
	}
	if strings.Contains(gemini.stdout, "Agent Platform") {
		t.Errorf("Gemini API のカタログに Agent Platform の状態が出ている")
	}

	machine := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "-o", "json", "--backend", "vertex", "--project", "my-proj", "models")
	if machine.err != nil || !strings.Contains(machine.stdout, "agent_platform") {
		t.Errorf("機械可読のカタログに agent_platform が無い: %v\n%s", machine.err, machine.stdout)
	}
}
