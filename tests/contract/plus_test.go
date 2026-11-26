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
	plusBinaryOnce sync.Once
	plusBinaryPath string
	plusBinaryErr  error
)

// buildPlusBinary は gmn を実行ファイル名どおりにビルドする。
func buildPlusBinary(t *testing.T) string {
	t.Helper()
	plusBinaryOnce.Do(func() {
		root, err := repoRoot()
		if err != nil {
			plusBinaryErr = err
			return
		}
		dir, err := os.MkdirTemp("", "gmn-contract-")
		if err != nil {
			plusBinaryErr = err
			return
		}
		plusBinaryPath = filepath.Join(dir, "gmn")
		cmd := exec.Command("go", "build", "-o", plusBinaryPath, "./cmd/gmn")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			plusBinaryErr = fmt.Errorf("build gmn: %v\n%s", err, out)
		}
	})
	if plusBinaryErr != nil {
		t.Fatal(plusBinaryErr)
	}
	return plusBinaryPath
}

func runPlus(t *testing.T, home string, env map[string]string, args ...string) commandResult {
	t.Helper()
	cmd := exec.Command(buildPlusBinary(t), args...)
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

func TestPlusバイナリ名がgmnでhelpが動く(t *testing.T) {
	bin := buildPlusBinary(t)
	if filepath.Base(bin) != "gmn" {
		t.Fatalf("実行ファイル名 = %q", filepath.Base(bin))
	}
	result := runPlus(t, t.TempDir(), nil, "--help")
	if result.err != nil {
		t.Fatalf("--help failed: %v\n%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "gmn") {
		t.Errorf("help にコマンド名 gmn が出ていない:\n%s", result.stdout)
	}
}

func TestPlus全コマンドのhelpとusageに本家のコマンド名が出ない(t *testing.T) {
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
			result := runPlus(t, home, nil, append(append([]string(nil), path...), flag)...)
			if result.err != nil {
				t.Errorf("%s %s failed: %v\n%s", name, flag, result.err, result.stderr)
				continue
			}
			assertNoUpstreamName(t, fmt.Sprintf("%q %s の stdout", name, flag), result.stdout)
			assertNoUpstreamName(t, fmt.Sprintf("%q %s の stderr", name, flag), result.stderr)
		}
	}
	helpGlobal := runPlus(t, home, nil, "--help-global")
	assertNoUpstreamName(t, "--help-global", helpGlobal.stdout)
}

func TestPlusエラーの案内もgmn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"Permission denied","status":"PERMISSION_DENIED"}}`)
	}))
	defer server.Close()

	t.Run("agent-mode の構造化エラー", func(t *testing.T) {
		result := runPlus(t, t.TempDir(), nil, "--agent-mode", "--server-url", server.URL, "--no-retries", "--api-key", "bad", "generate", "hi")
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
		result := runPlus(t, t.TempDir(), nil, "--no-interactive", "--server-url", server.URL, "--no-retries", "--api-key", "bad", "generate", "hi")
		if result.err == nil {
			t.Fatal("403 で失敗するはず")
		}
		if !strings.Contains(result.stderr, "gmn") {
			t.Errorf("案内に gmn が出ていない:\n%s", result.stderr)
		}
		assertNoUpstreamName(t, "通常のエラー", result.stderr)
	})

	t.Run("存在しないコマンドの案内", func(t *testing.T) {
		result := runPlus(t, t.TempDir(), nil, "nosuchcommand")
		assertNoUpstreamName(t, "不明コマンドのエラー", result.stderr)
	})
}

func TestPlus設定ファイルはgmn用で公式の設定を読まない(t *testing.T) {
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

	if result := runPlus(t, home, nil, generate...); result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if len(gotKeys) != 1 || gotKeys[0] == "OFFICIAL" {
		t.Fatalf("公式の設定の API key を読んでいる: %v", gotKeys)
	}

	if result := runPlus(t, home, nil, "--no-interactive", "--no-keyring", "configure", "--api-key", "FORK"); result.err != nil {
		t.Fatalf("configure failed: %v\n%s", result.err, result.stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "gmn", "config.yaml")); err != nil {
		t.Errorf("fork 用の設定ファイルが書かれていない: %v", err)
	}
	official, _ := os.ReadFile(filepath.Join(officialDir, "config.yaml"))
	if strings.Contains(string(official), "FORK") {
		t.Errorf("公式の設定ファイルに書き込んでいる:\n%s", official)
	}
	if result := runPlus(t, home, nil, generate...); result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if len(gotKeys) != 2 || gotKeys[1] != "FORK" {
		t.Errorf("fork 用の設定の API key を使っていない: %v", gotKeys)
	}
}

func TestPlusのversionにfork版と本家の版が出る(t *testing.T) {
	result := runPlus(t, t.TempDir(), nil, "--version")
	if result.err != nil {
		t.Fatalf("--version failed: %v\n%s", result.err, result.stderr)
	}
	if !regexp.MustCompile(`^gmn 0\.1\.0\S* \(base gemini-api-cli 0\.5\.0\)\n`).MatchString(result.stdout) {
		t.Errorf("--version = %q", result.stdout)
	}

	machine := runPlus(t, t.TempDir(), nil, "version", "-o", "json")
	var info map[string]string
	if err := json.Unmarshal([]byte(machine.stdout), &info); err != nil {
		t.Fatalf("version -o json が JSON ではない: %v\n%s", err, machine.stdout)
	}
	if info["name"] != "gmn" || !strings.HasPrefix(info["version"], "0.1.0") || info["base_version"] != "0.5.0" {
		t.Errorf("version -o json = %v", info)
	}
}

func TestPlusのUserAgentにfork名が入る(t *testing.T) {
	agents := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents <- r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"i","status":"completed"}`)
	}))
	defer server.Close()

	args := append(baseArgs(server.URL), "--api-key", "k", "generate", "hi", "--stream=false")
	if result := runPlus(t, t.TempDir(), nil, args...); result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if ua := <-agents; !strings.HasPrefix(ua, "gmn/0.1.0") || !strings.Contains(ua, "google-genai-cli/") {
		t.Errorf("User-Agent = %q", ua)
	}
}

// モデルの返答に含まれる gemini-api は書き換えない（置き換えは案内の文面だけ）。
func TestPlusはモデルの返答の中の文字列を書き換えない(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"i","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"run gemini-api configure"}]}]}`)
	}))
	defer server.Close()

	args := append(baseArgs(server.URL), "--api-key", "k", "generate", "hi", "--stream=false")
	result := runPlus(t, t.TempDir(), nil, args...)
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "run gemini-api configure") {
		t.Errorf("返答が書き換えられている:\n%s", result.stdout)
	}
}
