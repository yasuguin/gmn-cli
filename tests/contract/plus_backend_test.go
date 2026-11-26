package contract_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gemini-headless の接続先（backend）の契約。Gemini API と Agent Platform（旧 Vertex AI）の切り替えを固定する。

// dryRunURL は generate --dry-run が示すリクエスト URL を返す。
func dryRunURL(t *testing.T, home string, env map[string]string, args ...string) string {
	t.Helper()
	full := append([]string{"--no-interactive", "--no-keyring", "--dry-run", "-o", "json"}, args...)
	full = append(full, "generate", "hi")
	result := runPlus(t, home, env, full...)
	if result.err != nil {
		t.Fatalf("generate --dry-run failed: %v\n%s", result.err, result.stderr)
	}
	var parsed struct {
		Request struct {
			URL     string              `json:"url"`
			Headers map[string][]string `json:"headers"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("dry-run の出力が JSON ではない: %v\n%s", err, result.stdout)
	}
	return parsed.Request.URL
}

func writePlusConfig(t *testing.T, home, body string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "gemini-headless")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPlusBackend何も設定しないとGeminiAPIに送る(t *testing.T) {
	got := dryRunURL(t, t.TempDir(), nil, "--api-key", "k")
	if got != "https://generativelanguage.googleapis.com/v1beta/interactions" {
		t.Errorf("url = %s", got)
	}
}

func TestPlusBackendVertexのglobalはaiplatformのホストに向く(t *testing.T) {
	got := dryRunURL(t, t.TempDir(), nil, "--backend", "vertex", "--project", "my-proj")
	want := "https://aiplatform.googleapis.com/v1beta1/projects/my-proj/locations/global/interactions"
	if got != want {
		t.Errorf("url = %s, want %s", got, want)
	}
}

func TestPlusBackendリージョンならホスト名にlocationが入る(t *testing.T) {
	got := dryRunURL(t, t.TempDir(), nil, "--backend", "vertex", "--project", "my-proj", "--location", "asia-northeast1")
	want := "https://asia-northeast1-aiplatform.googleapis.com/v1beta1/projects/my-proj/locations/asia-northeast1/interactions"
	if got != want {
		t.Errorf("url = %s, want %s", got, want)
	}
}

func TestPlusBackend環境変数だけで切り替わる(t *testing.T) {
	env := map[string]string{
		"GOOGLE_GENAI_USE_VERTEXAI": "true",
		"GOOGLE_CLOUD_PROJECT":      "env-proj",
		"GOOGLE_CLOUD_LOCATION":     "us-central1",
	}
	got := dryRunURL(t, t.TempDir(), env)
	want := "https://us-central1-aiplatform.googleapis.com/v1beta1/projects/env-proj/locations/us-central1/interactions"
	if got != want {
		t.Errorf("url = %s, want %s", got, want)
	}
	// 1 でも有効
	env["GOOGLE_GENAI_USE_VERTEXAI"] = "1"
	if got := dryRunURL(t, t.TempDir(), env); got != want {
		t.Errorf("USE_VERTEXAI=1 の url = %s", got)
	}
}

func TestPlusBackend設定ファイルだけで切り替わる(t *testing.T) {
	home := t.TempDir()
	writePlusConfig(t, home, "backend: vertex\nproject: cfg-proj\nlocation: europe-west4\n")
	got := dryRunURL(t, home, nil)
	want := "https://europe-west4-aiplatform.googleapis.com/v1beta1/projects/cfg-proj/locations/europe-west4/interactions"
	if got != want {
		t.Errorf("url = %s, want %s", got, want)
	}
}

func TestPlusBackendフラグは環境変数に環境変数は設定に勝つ(t *testing.T) {
	home := t.TempDir()
	writePlusConfig(t, home, "backend: vertex\nproject: cfg-proj\nlocation: europe-west4\n")

	env := map[string]string{"GOOGLE_CLOUD_PROJECT": "env-proj", "GOOGLE_CLOUD_LOCATION": "us-central1"}
	got := dryRunURL(t, home, env)
	if want := "https://us-central1-aiplatform.googleapis.com/v1beta1/projects/env-proj/locations/us-central1/interactions"; got != want {
		t.Errorf("環境変数が設定に勝つはず: %s", got)
	}

	got = dryRunURL(t, home, env, "--project", "flag-proj", "--location", "global")
	if want := "https://aiplatform.googleapis.com/v1beta1/projects/flag-proj/locations/global/interactions"; got != want {
		t.Errorf("フラグが環境変数に勝つはず: %s", got)
	}

	// 設定が vertex でも、フラグで gemini に戻せる
	got = dryRunURL(t, home, nil, "--backend", "gemini", "--api-key", "k")
	if got != "https://generativelanguage.googleapis.com/v1beta/interactions" {
		t.Errorf("--backend gemini が設定に勝つはず: %s", got)
	}
	// 環境変数が false なら設定の vertex に勝つ
	got = dryRunURL(t, home, map[string]string{"GOOGLE_GENAI_USE_VERTEXAI": "false"}, "--api-key", "k")
	if got != "https://generativelanguage.googleapis.com/v1beta/interactions" {
		t.Errorf("USE_VERTEXAI=false が設定に勝つはず: %s", got)
	}
}

func TestPlusBackendVertexはAPIキーを送らずBearerとuserProjectを送る(t *testing.T) {
	var gotHeaders http.Header
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"i","status":"completed"}`)
	}))
	defer server.Close()

	// API key が環境変数・フラグのどちらにあっても送らない
	result := runPlus(t, t.TempDir(), map[string]string{"GEMINI_API_KEY": "SECRETKEY"},
		"--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--access-token", "tok-123", "--api-key", "SECRETKEY2",
		"--server-url", server.URL, "generate", "hi", "--stream=false")
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if got := gotHeaders.Get("Authorization"); got != "Bearer tok-123" {
		t.Errorf("Authorization = %q", got)
	}
	if got := gotHeaders.Get("x-goog-user-project"); got != "my-proj" {
		t.Errorf("x-goog-user-project = %q", got)
	}
	if got := gotHeaders.Get("x-goog-api-key"); got != "" {
		t.Errorf("x-goog-api-key を送ってはいけない: %q", got)
	}
	// --server-url 明示ならスタブの URL にそのまま送る（Agent Platform の URL に書き換えない）
	if !strings.HasSuffix(gotPath, "/interactions") {
		t.Errorf("path = %s", gotPath)
	}
}

func TestPlusBackend明示のuserProjectはprojectより優先する(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("x-goog-user-project")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"i","status":"completed"}`)
	}))
	defer server.Close()
	result := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--user-project", "billing-proj", "--access-token", "t",
		"--server-url", server.URL, "generate", "hi", "--stream=false")
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if got != "billing-proj" {
		t.Errorf("x-goog-user-project = %q", got)
	}
}

func TestPlusBackendGeminiAPIではAPIキーを送りBearerを送らない(t *testing.T) {
	var gotHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"i","status":"completed"}`)
	}))
	defer server.Close()
	result := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--api-key", "K1", "--server-url", server.URL, "generate", "hi", "--stream=false")
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if gotHeaders.Get("x-goog-api-key") != "K1" || gotHeaders.Get("Authorization") != "" {
		t.Errorf("headers = %v", gotHeaders)
	}
}

func TestPlusBackendVertexでprojectが無いと設定方法を示して終了コード2(t *testing.T) {
	result := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--backend", "vertex", "generate", "hi", "--dry-run")
	if result.err == nil {
		t.Fatal("project が無ければ失敗するはず")
	}
	if code := exitCodeOf(result.err); code != 2 {
		t.Errorf("終了コード = %d, want 2\n%s", code, result.stderr)
	}
	for _, want := range []string{"--project", "GOOGLE_CLOUD_PROJECT", "gemini-headless"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("エラーに %q が無い:\n%s", want, result.stderr)
		}
	}
}

func TestPlusBackend不正なbackendは終了コード2(t *testing.T) {
	result := runPlus(t, t.TempDir(), nil, "--no-interactive", "--backend", "nope", "whoami")
	if result.err == nil || exitCodeOf(result.err) != 2 {
		t.Fatalf("err = %v, stderr = %s", result.err, result.stderr)
	}
}

func TestPlusWhoamiがbackendとprojectとlocationを表示する(t *testing.T) {
	result := runPlus(t, t.TempDir(), map[string]string{"GOOGLE_CLOUD_PROJECT": "env-proj"},
		"--no-interactive", "--no-keyring", "--backend", "vertex", "whoami")
	if result.err != nil {
		t.Fatalf("whoami failed: %v\n%s", result.err, result.stderr)
	}
	for _, want := range []string{"vertex", "env-proj", "global"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("whoami に %q が無い:\n%s", want, result.stdout)
		}
	}

	machine := runPlus(t, t.TempDir(), map[string]string{"GOOGLE_CLOUD_PROJECT": "env-proj"},
		"--no-interactive", "--no-keyring", "--backend", "vertex", "-o", "json", "whoami")
	var parsed struct {
		Backend map[string]map[string]any `json:"backend"`
	}
	if err := json.Unmarshal([]byte(machine.stdout), &parsed); err != nil {
		t.Fatalf("whoami -o json が JSON ではない: %v\n%s", err, machine.stdout)
	}
	if parsed.Backend["backend"]["value"] != "vertex" || parsed.Backend["project"]["value"] != "env-proj" || parsed.Backend["location"]["value"] != "global" {
		t.Errorf("backend = %v", parsed.Backend)
	}
}

func TestPlusWhoamiは既定でgeminiを表示する(t *testing.T) {
	result := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "whoami")
	if result.err != nil {
		t.Fatalf("whoami failed: %v\n%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "gemini") {
		t.Errorf("whoami に backend が無い:\n%s", result.stdout)
	}
}

func TestPlus本家のgemini_apiにはbackendフラグが無い(t *testing.T) {
	result := runCLI(t, t.TempDir(), nil, "--no-interactive", "--backend", "vertex", "whoami")
	if result.err == nil {
		t.Fatal("本家に --backend が生えている")
	}
}

func exitCodeOf(err error) int {
	if ee, ok := err.(interface{ ExitCode() int }); ok {
		return ee.ExitCode()
	}
	return -1
}

func TestPlusBackendVertexでprojectが無くてもローカルのコマンドは動く(t *testing.T) {
	for _, args := range [][]string{{"whoami"}, {"version"}, {"configure", "--help"}, {"generate", "--help"}} {
		full := append([]string{"--no-interactive", "--no-keyring", "--backend", "vertex"}, args...)
		result := runPlus(t, t.TempDir(), nil, full...)
		if result.err != nil {
			t.Errorf("%v: project 未設定でも動くはず: %v\n%s", args, result.err, result.stderr)
		}
	}
	result := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--backend", "vertex", "whoami")
	if !strings.Contains(result.stdout, "vertex") || !strings.Contains(result.stdout, "unset") {
		t.Errorf("whoami が project 未設定を表示していない:\n%s", result.stdout)
	}
}
