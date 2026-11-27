package contract_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Agent Platform 向けの案内と、未対応コマンドの停止の契約。

// countingStub はリクエスト数を数える。通信しないことの確認に使う。
func countingStub(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	server := httptest.NewServer(onlyFromCLI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	})))
	t.Cleanup(server.Close)
	return server, &n
}

var capVertexArgs = []string{"--no-interactive", "--no-keyring", "--no-retries", "--backend", "vertex", "--project", "my-proj"}

func TestGmn未対応コマンドはAgentPlatformで通信せずに終了コード2で止まる(t *testing.T) {
	server, n := countingStub(t, 200, `{}`)
	for _, path := range [][]string{
		{"environments", "list"},
		{"environments", "files", "list", "--environment", "e"},
		{"triggers", "list"},
		{"webhooks", "list"},
		{"voices", "list"},
		{"credentials", "list"},
		{"files", "list"},
	} {
		args := append(append([]string{}, capVertexArgs...), "--server-url", server.URL)
		args = append(args, path...)
		result := runGmn(t, t.TempDir(), nil, args...)
		name := strings.Join(path, " ")
		if result.err == nil || exitCodeOf(result.err) != 2 {
			t.Errorf("%s: 終了コード = %d, err = %v\n%s", name, exitCodeOf(result.err), result.err, result.stderr)
			continue
		}
		for _, want := range []string{"not available on Agent Platform", "--backend gemini"} {
			if !strings.Contains(result.stderr, want) {
				t.Errorf("%s: エラーに %q が無い:\n%s", name, want, result.stderr)
			}
		}
	}
	if got := n.Load(); got != 0 {
		t.Errorf("スタブにリクエストが %d 件届いた（0 件のはず）", got)
	}
}

func TestGmn未対応コマンドでもGeminiAPIでは本家のまま送る(t *testing.T) {
	server, n := countingStub(t, 200, `{}`)
	result := runGmn(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--no-retries", "--api-key", "k",
		"--server-url", server.URL, "triggers", "list")
	if result.err != nil {
		t.Fatalf("triggers list failed: %v\n%s", result.err, result.stderr)
	}
	if n.Load() != 1 {
		t.Errorf("リクエスト数 = %d, want 1", n.Load())
	}
}

func TestGmnfilesUploadはgsのURIを渡す方法を示して止まる(t *testing.T) {
	server, n := countingStub(t, 200, `{}`)
	file := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := append(append([]string{}, capVertexArgs...), "--server-url", server.URL, "files", "upload", file)
	result := runGmn(t, t.TempDir(), nil, args...)
	if result.err == nil || exitCodeOf(result.err) != 2 {
		t.Fatalf("終了コード = %d, err = %v\n%s", exitCodeOf(result.err), result.err, result.stderr)
	}
	for _, want := range []string{"gs://", "--input"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("エラーに %q が無い:\n%s", want, result.stderr)
		}
	}
	if n.Load() != 0 {
		t.Errorf("リクエスト数 = %d, want 0", n.Load())
	}
}

func TestGmnAgentModeの構造化エラーにAgentPlatform用の直し方が入る(t *testing.T) {
	args := append(append([]string{}, capVertexArgs...), "--agent-mode", "-o", "json", "webhooks", "list")
	result := runGmn(t, t.TempDir(), nil, args...)
	if result.err == nil || exitCodeOf(result.err) != 2 {
		t.Fatalf("終了コード = %d, err = %v\n%s", exitCodeOf(result.err), result.err, result.stderr)
	}
	text := result.stderr + result.stdout
	var parsed struct {
		ErrorType string   `json:"error_type"`
		ExitCode  int      `json:"exit_code"`
		Hints     []string `json:"hints"`
	}
	start := strings.Index(text, "{")
	if start < 0 || json.Unmarshal([]byte(text[start:]), &parsed) != nil {
		t.Fatalf("構造化エラーが JSON ではない:\n%s", text)
	}
	if parsed.ExitCode != 2 || parsed.ErrorType != "validation_error" {
		t.Errorf("error_type = %q, exit_code = %d", parsed.ErrorType, parsed.ExitCode)
	}
	joined := strings.Join(parsed.Hints, "\n")
	for _, want := range []string{"--backend gemini", "GOOGLE_GENAI_USE_VERTEXAI=false"} {
		if !strings.Contains(joined, want) {
			t.Errorf("hints に %q が無い:\n%s", want, joined)
		}
	}
}

// API key を求める案内の目印。Agent Platform の出力には出てはいけない。
var apiKeyAsks = []string{"No API key", "GEMINI_API_KEY", "aistudio.google.com/docs/api-key", "must be created manually"}

func assertNoAPIKeyAsk(t *testing.T, label, text string) {
	t.Helper()
	assertNone(t, label, text, apiKeyAsks)
}

// ヘルプと --usage には --api-key フラグの説明（env: GEMINI_API_KEY）が正当に載るので、案内の文言だけを見る。
var apiKeyAsksInDocs = []string{"No API key", "Set GEMINI_API_KEY", "Setup: export GEMINI_API_KEY", "must be created manually", "Stop and ask the user to create"}

func assertNone(t *testing.T, label, text string, bads []string) {
	t.Helper()
	for _, bad := range bads {
		if strings.Contains(text, bad) {
			t.Errorf("%s に API key を求める案内 %q が出ている:\n%s", label, bad, text)
		}
	}
}

func TestGmnAgentPlatformのヘルプはAPIキーを求めずADCを案内する(t *testing.T) {
	env := map[string]string{"GOOGLE_GENAI_USE_VERTEXAI": "true"}
	for _, args := range [][]string{{"--help"}, {"--agent-mode", "--help"}, {"--usage"}} {
		// project 未設定でも、API key ではなく project の設定方法を示す
		result := runGmn(t, t.TempDir(), env, append([]string{"--no-keyring"}, args...)...)
		if result.err != nil {
			t.Fatalf("%v failed: %v\n%s", args, result.err, result.stderr)
		}
		label := strings.Join(args, " ")
		assertNone(t, label, result.stdout, apiKeyAsksInDocs)
		for _, want := range []string{"gcloud auth application-default login", "GOOGLE_CLOUD_PROJECT"} {
			if !strings.Contains(result.stdout, want) {
				t.Errorf("%s に %q が無い:\n%s", label, want, result.stdout)
			}
		}
	}
}

func TestGmnGeminiAPIのヘルプは本家のAPIキー案内のまま(t *testing.T) {
	result := runGmn(t, t.TempDir(), nil, "--no-keyring", "--help")
	if result.err != nil {
		t.Fatal(result.err)
	}
	for _, want := range []string{"No API key found", "Set GEMINI_API_KEY, or run: gmn configure", "Stop and ask the user"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("help に %q が無い:\n%s", want, result.stdout)
		}
	}
}

func TestGmnAgentPlatformの認証エラーはAPIキーを求めずADCを案内する(t *testing.T) {
	body := `{"error":{"code":401,"message":"missing credential","status":"UNAUTHENTICATED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"CREDENTIALS_MISSING","domain":"googleapis.com"}]}}`
	server, _ := countingStub(t, 401, body)
	args := append(append([]string{}, capVertexArgs...), "--agent-mode", "-o", "json", "--server-url", server.URL, "generate", "hi", "--stream=false")
	result := runGmn(t, t.TempDir(), map[string]string{"GEMINI_API_KEY": "SECRETKEY"}, args...)
	if result.err == nil || exitCodeOf(result.err) != 3 {
		t.Fatalf("終了コード = %d, err = %v\n%s", exitCodeOf(result.err), result.err, result.stderr)
	}
	text := result.stderr + result.stdout
	assertNoAPIKeyAsk(t, "認証エラー", text)
	for _, want := range []string{"gcloud auth application-default login", "AI agents:"} {
		if !strings.Contains(text, want) {
			t.Errorf("認証エラーに %q が無い:\n%s", want, text)
		}
	}
}

func TestGmnGeminiAPIの認証エラーは本家のAPIキー案内のまま(t *testing.T) {
	body := `{"error":{"code":401,"message":"missing credential","status":"UNAUTHENTICATED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"CREDENTIALS_MISSING","domain":"googleapis.com"}]}}`
	server, _ := countingStub(t, 401, body)
	result := runGmn(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--no-retries", "--agent-mode", "-o", "json",
		"--server-url", server.URL, "generate", "hi", "--stream=false")
	text := result.stderr + result.stdout
	if !strings.Contains(text, "GEMINI_API_KEY") || !strings.Contains(text, "Stop and ask the user") {
		t.Errorf("本家の案内が出ていない:\n%s", text)
	}
}

func TestGmnUsageとhelpGlobalにbackendの指定方法が出る(t *testing.T) {
	for _, flag := range []string{"--usage", "--help-global"} {
		result := runGmn(t, t.TempDir(), nil, "--no-keyring", flag)
		if result.err != nil {
			t.Fatalf("%s failed: %v\n%s", flag, result.err, result.stderr)
		}
		for _, want := range []string{"--backend", "--project", "--location",
			"GOOGLE_GENAI_USE_VERTEXAI", "GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION", "config"} {
			if !strings.Contains(result.stdout, want) {
				t.Errorf("%s に %q が無い", flag, want)
			}
		}
	}
}
