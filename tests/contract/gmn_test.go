package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google-gemini/gemini-api-cli/internal/cli"
	"github.com/spf13/cobra"
)

// fork 用 CLI（gmn）の契約。公式の gemini-api と並べて入れても混ざらないことを固定する。

var (
	gmnBinaryOnce sync.Once
	gmnBinaryPath string
	gmnBinaryErr  error
)

// buildGmnBinary は gmn を実行ファイル名どおりにビルドする。
func buildGmnBinary(t *testing.T) string {
	t.Helper()
	gmnBinaryOnce.Do(func() {
		root, err := repoRoot()
		if err != nil {
			gmnBinaryErr = err
			return
		}
		dir, err := os.MkdirTemp("", "gmn-contract-")
		if err != nil {
			gmnBinaryErr = err
			return
		}
		gmnBinaryPath = filepath.Join(dir, "gmn")
		cmd := exec.Command("go", "build", "-o", gmnBinaryPath, "./cmd/gmn")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			gmnBinaryErr = fmt.Errorf("build gmn: %v\n%s", err, out)
		}
	})
	if gmnBinaryErr != nil {
		t.Fatal(gmnBinaryErr)
	}
	return gmnBinaryPath
}

func runGmn(t *testing.T, home string, env map[string]string, args ...string) commandResult {
	t.Helper()
	cmd := exec.Command(buildGmnBinary(t), args...)
	cmd.Env = isolatedEnv(home, env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// bareUpstreamName は gmn・gemini-api-cli・URL・パスの一部ではない、
// 単独のコマンド名 gemini-api を探す。
var bareUpstreamName = regexp.MustCompile(`(^|[^A-Za-z0-9_./-])gemini-api($|[^A-Za-z0-9_/-])`)

func assertNoUpstreamName(t *testing.T, label, text string) {
	t.Helper()
	if loc := bareUpstreamName.FindStringIndex(text); loc != nil {
		start := max(loc[0]-40, 0)
		end := min(loc[1]+40, len(text))
		t.Errorf("%s に gemini-api というコマンド名が残っている: ...%s...", label, text[start:end])
	}
}

func allCommandPaths(c *cobra.Command, prefix []string, out *[][]string) {
	for _, child := range c.Commands() {
		path := append(append([]string(nil), prefix...), child.Name())
		*out = append(*out, path)
		allCommandPaths(child, path, out)
	}
}

func TestGmnバイナリ名がgmnでhelpが動く(t *testing.T) {
	bin := buildGmnBinary(t)
	if filepath.Base(bin) != "gmn" {
		t.Fatalf("実行ファイル名 = %q", filepath.Base(bin))
	}
	result := runGmn(t, t.TempDir(), nil, "--help")
	if result.err != nil {
		t.Fatalf("--help failed: %v\n%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "gmn") {
		t.Errorf("help にコマンド名 gmn が出ていない:\n%s", result.stdout)
	}
}

func TestGmn全コマンドのhelpとusageに本家のコマンド名が出ない(t *testing.T) {
	root, err := cli.NewRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	paths := [][]string{{}}
	allCommandPaths(root, nil, &paths)
	if len(paths) < 50 {
		t.Fatalf("コマンドの走査数が少なすぎる: %d", len(paths))
	}
	home := t.TempDir()
	for _, path := range paths {
		name := strings.Join(path, " ")
		for _, flag := range []string{"--help", "--usage"} {
			result := runGmn(t, home, nil, append(append([]string(nil), path...), flag)...)
			if result.err != nil {
				t.Errorf("%s %s failed: %v\n%s", name, flag, result.err, result.stderr)
				continue
			}
			assertNoUpstreamName(t, fmt.Sprintf("%q %s の stdout", name, flag), result.stdout)
			assertNoUpstreamName(t, fmt.Sprintf("%q %s の stderr", name, flag), result.stderr)
		}
	}
	helpGlobal := runGmn(t, home, nil, "--help-global")
	assertNoUpstreamName(t, "--help-global", helpGlobal.stdout)
}

func TestGmnエラーの案内もgmn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"Permission denied","status":"PERMISSION_DENIED"}}`)
	}))
	defer server.Close()

	t.Run("agent-mode の構造化エラー", func(t *testing.T) {
		result := runGmn(t, t.TempDir(), nil, "--agent-mode", "--server-url", server.URL, "--no-retries", "--api-key", "bad", "generate", "hi")
		if result.err == nil {
			t.Fatal("403 で失敗するはず")
		}
		var parsed struct {
			Hints []string `json:"hints"`
		}
		if err := json.Unmarshal([]byte(result.stderr), &parsed); err != nil {
			t.Fatalf("構造化エラーが JSON ではない: %v\n%s", err, result.stderr)
		}
		if !strings.Contains(strings.Join(parsed.Hints, "\n"), "gmn whoami") {
			t.Errorf("hints に gmn whoami が無い: %v", parsed.Hints)
		}
		assertNoUpstreamName(t, "agent-mode のエラー", result.stderr)
	})

	t.Run("通常のエラーの案内", func(t *testing.T) {
		result := runGmn(t, t.TempDir(), nil, "--no-interactive", "--server-url", server.URL, "--no-retries", "--api-key", "bad", "generate", "hi")
		if result.err == nil {
			t.Fatal("403 で失敗するはず")
		}
		if !strings.Contains(result.stderr, "gmn") {
			t.Errorf("案内に gmn が出ていない:\n%s", result.stderr)
		}
		assertNoUpstreamName(t, "通常のエラー", result.stderr)
	})

	t.Run("存在しないコマンドの案内", func(t *testing.T) {
		result := runGmn(t, t.TempDir(), nil, "nosuchcommand")
		assertNoUpstreamName(t, "不明コマンドのエラー", result.stderr)
	})
}

func TestGmn設定ファイルはgmn用で公式の設定を読まない(t *testing.T) {
	var gotKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKeys = append(gotKeys, r.Header.Get("x-goog-api-key"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"i","status":"completed"}`)
	}))
	defer server.Close()

	home := t.TempDir()
	officialDir := filepath.Join(home, ".config", "gemini-api")
	if err := os.MkdirAll(officialDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(officialDir, "config.yaml"), []byte("security:\n  api_key: OFFICIAL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	generate := []string{"--server-url", server.URL, "--no-interactive", "--no-retries", "--no-keyring", "--output-format", "json", "generate", "hi", "--stream=false"}

	if result := runGmn(t, home, nil, generate...); result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if len(gotKeys) != 1 || gotKeys[0] == "OFFICIAL" {
		t.Fatalf("公式の設定の API key を読んでいる: %v", gotKeys)
	}

	if result := runGmn(t, home, nil, "--no-interactive", "--no-keyring", "configure", "--api-key", "FORK"); result.err != nil {
		t.Fatalf("configure failed: %v\n%s", result.err, result.stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "gmn", "config.yaml")); err != nil {
		t.Errorf("fork 用の設定ファイルが書かれていない: %v", err)
	}
	official, _ := os.ReadFile(filepath.Join(officialDir, "config.yaml"))
	if strings.Contains(string(official), "FORK") {
		t.Errorf("公式の設定ファイルに書き込んでいる:\n%s", official)
	}
	if result := runGmn(t, home, nil, generate...); result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if len(gotKeys) != 2 || gotKeys[1] != "FORK" {
		t.Errorf("fork 用の設定の API key を使っていない: %v", gotKeys)
	}
}

func TestGmnのversionにfork版と本家の版が出る(t *testing.T) {
	result := runGmn(t, t.TempDir(), nil, "--version")
	if result.err != nil {
		t.Fatalf("--version failed: %v\n%s", result.err, result.stderr)
	}
	if !regexp.MustCompile(`^gmn 0\.1\.0\S* \(base gemini-api-cli 0\.5\.0\)\n`).MatchString(result.stdout) {
		t.Errorf("--version = %q", result.stdout)
	}

	machine := runGmn(t, t.TempDir(), nil, "version", "-o", "json")
	var info map[string]string
	if err := json.Unmarshal([]byte(machine.stdout), &info); err != nil {
		t.Fatalf("version -o json が JSON ではない: %v\n%s", err, machine.stdout)
	}
	if info["name"] != "gmn" || !strings.HasPrefix(info["version"], "0.1.0") || info["base_version"] != "0.5.0" {
		t.Errorf("version -o json = %v", info)
	}
}

func TestGmnのUserAgentにfork名が入る(t *testing.T) {
	agents := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents <- r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"i","status":"completed"}`)
	}))
	defer server.Close()

	args := append(baseArgs(server.URL), "--api-key", "k", "generate", "hi", "--stream=false")
	if result := runGmn(t, t.TempDir(), nil, args...); result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if ua := <-agents; !strings.HasPrefix(ua, "gmn/0.1.0") || !strings.Contains(ua, "google-genai-cli/") {
		t.Errorf("User-Agent = %q", ua)
	}
}

// モデルの返答に含まれる gemini-api は書き換えない（置き換えは案内の文面だけ）。
func TestGmnはモデルの返答の中の文字列を書き換えない(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"i","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"run gemini-api configure"}]}]}`)
	}))
	defer server.Close()

	args := append(baseArgs(server.URL), "--api-key", "k", "generate", "hi", "--stream=false")
	result := runGmn(t, t.TempDir(), nil, args...)
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "run gemini-api configure") {
		t.Errorf("返答が書き換えられている:\n%s", result.stdout)
	}
}

// onlyFromCLI は、CLI（gmn・gemini-api）と Go の HTTP クライアント（ADC のトークン取得など）から来たリクエストだけを h に渡し、
// ほかは 404 で返して数えない。手元では localhost の待ち受けを見つけて叩くツール（開発サーバーの探索など）の
// リクエストがスタブに届くことがあり、それを数えると件数や中身の確かめが回ごとに揺れるため。
func onlyFromCLI(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !fromCLI(r) {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func fromCLI(r *http.Request) bool {
	ua := r.UserAgent()
	for _, prefix := range []string{"gmn/", "gemini-api/", "google-genai-cli/", "Go-http-client/"} {
		if strings.HasPrefix(ua, prefix) {
			return true
		}
	}
	return false
}

func TestGmnスタブはCLI以外のプロセスからのリクエストを数えない(t *testing.T) {
	stub := newInteractionStub(t, gcsOKResponse)
	req, err := http.NewRequest(http.MethodGet, stub.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "undici") // Node.js の fetch。手元の開発サーバーの探索がこの形で叩いてくる
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
	if n := stub.requests.Load(); n != 0 {
		t.Errorf("CLI 以外からのリクエストを %d 回数えた", n)
	}
}
