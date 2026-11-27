package contract_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// gmn が Agent Platform で ADC（Application Default Credentials）から
// アクセストークンを取る契約。実際の Google には接続せず、token_uri をスタブに向ける。

// tokenStub は OAuth のトークンエンドポイントのスタブ。呼ばれた回数を数え、tok-1, tok-2... を返す。
type tokenStub struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  int
}

func newTokenStub(t *testing.T) *tokenStub {
	t.Helper()
	s := &tokenStub{}
	s.server = httptest.NewServer(onlyFromCLI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls++
		n := s.calls
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok-%d","token_type":"Bearer","expires_in":3600}`, n)
	})))
	t.Cleanup(s.server.Close)
	return s
}

func (s *tokenStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// writeServiceAccountKey はテスト内で作った RSA 鍵の service account 形式 JSON を書き、そのパスを返す。
func writeServiceAccountKey(t *testing.T, tokenURI string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "stub-proj",
		"private_key_id": "kid",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email":   "sa@stub-proj.iam.gserviceaccount.com",
		"client_id":      "1",
		"token_uri":      tokenURI,
	})
	path := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// apiStub は Agent Platform の代わりに受けたリクエストのヘッダーを記録する。
func apiStub(t *testing.T) (*httptest.Server, func() []http.Header) {
	t.Helper()
	var mu sync.Mutex
	var got []http.Header
	server := httptest.NewServer(onlyFromCLI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Clone())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"i","status":"completed"}`)
	})))
	t.Cleanup(server.Close)
	return server, func() []http.Header {
		mu.Lock()
		defer mu.Unlock()
		return append([]http.Header(nil), got...)
	}
}

func vertexGenerate(server *httptest.Server, extra ...string) []string {
	args := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--server-url", server.URL}
	args = append(args, extra...)
	return append(args, "generate", "hi", "--stream=false")
}

func TestGmnADCはGOOGLE_APPLICATION_CREDENTIALSのトークンをBearerで送る(t *testing.T) {
	tokens := newTokenStub(t)
	api, headers := apiStub(t)
	creds := writeServiceAccountKey(t, tokens.server.URL)

	result := runGmn(t, t.TempDir(), map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": creds}, vertexGenerate(api)...)
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	got := headers()
	if len(got) != 1 || got[0].Get("Authorization") != "Bearer tok-1" {
		t.Fatalf("Authorization = %v", got)
	}
	if got[0].Get("x-goog-user-project") != "my-proj" {
		t.Errorf("x-goog-user-project = %q", got[0].Get("x-goog-user-project"))
	}
	if strings.Contains(result.stdout+result.stderr, "tok-1") {
		t.Errorf("出力にトークンが漏れている:\n%s\n%s", result.stdout, result.stderr)
	}
}

func TestGmnADCは明示のaccess_tokenがあれば読まない(t *testing.T) {
	cases := map[string]struct {
		env   map[string]string
		extra []string
	}{
		"フラグ":  {nil, []string{"--access-token", "explicit"}},
		"環境変数": {map[string]string{"GEMINI_ACCESS_TOKEN": "explicit"}, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			tokens := newTokenStub(t)
			api, headers := apiStub(t)
			env := map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": writeServiceAccountKey(t, tokens.server.URL)}
			for k, v := range c.env {
				env[k] = v
			}
			result := runGmn(t, t.TempDir(), env, vertexGenerate(api, c.extra...)...)
			if result.err != nil {
				t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
			}
			if got := headers(); len(got) != 1 || got[0].Get("Authorization") != "Bearer explicit" {
				t.Errorf("Authorization = %v", got)
			}
			if n := tokens.callCount(); n != 0 {
				t.Errorf("トークンエンドポイントが %d 回呼ばれた", n)
			}
		})
	}
}

func TestGmnADCは設定ファイルのaccess_tokenがあれば読まない(t *testing.T) {
	tokens := newTokenStub(t)
	api, headers := apiStub(t)
	home := t.TempDir()
	writeGmnConfig(t, home, "security:\n  access_token: from-config\n")
	result := runGmn(t, home, map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": writeServiceAccountKey(t, tokens.server.URL)}, vertexGenerate(api)...)
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if got := headers(); len(got) != 1 || got[0].Get("Authorization") != "Bearer from-config" {
		t.Errorf("Authorization = %v", got)
	}
	if n := tokens.callCount(); n != 0 {
		t.Errorf("トークンエンドポイントが %d 回呼ばれた", n)
	}
}

func TestGmnADCが無いと直し方を示して終了コード3(t *testing.T) {
	api, headers := apiStub(t)
	home := t.TempDir()
	started := time.Now()
	result := runGmn(t, home, map[string]string{"CLOUDSDK_CONFIG": filepath.Join(home, "gcloud")}, vertexGenerate(api)...)
	t.Logf("ADC 探索にかかった時間: %s", time.Since(started))
	if result.err == nil {
		t.Fatal("ADC が無ければ失敗するはず")
	}
	if code := exitCodeOf(result.err); code != 3 {
		t.Errorf("終了コード = %d, want 3\n%s", code, result.stderr)
	}
	for _, want := range []string{"gcloud auth application-default login", "GOOGLE_APPLICATION_CREDENTIALS"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("エラーに %q が無い:\n%s", want, result.stderr)
		}
	}
	if len(headers()) != 0 {
		t.Error("認証できないのにリクエストを送っている")
	}
}

func TestGmnADCはdry_runで読まず資格情報を伏せて表示する(t *testing.T) {
	tokens := newTokenStub(t)
	env := map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": writeServiceAccountKey(t, tokens.server.URL)}
	args := []string{"--no-interactive", "--no-keyring", "--dry-run", "-o", "json", "--backend", "vertex", "--project", "my-proj", "generate", "hi"}

	result := runGmn(t, t.TempDir(), env, args...)
	if result.err != nil {
		t.Fatalf("dry-run failed: %v\n%s", result.err, result.stderr)
	}
	if n := tokens.callCount(); n != 0 {
		t.Errorf("dry-run でトークンエンドポイントが %d 回呼ばれた", n)
	}
	var parsed struct {
		Request struct {
			Headers map[string][]string `json:"headers"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("dry-run の出力が JSON ではない: %v\n%s", err, result.stdout)
	}
	if auth := parsed.Request.Headers["Authorization"]; len(auth) != 1 || strings.Contains(auth[0], "tok-") {
		t.Errorf("Authorization = %v", auth)
	}

	// ADC が無い環境でも dry-run は通る
	if bare := runGmn(t, t.TempDir(), nil, args...); bare.err != nil {
		t.Errorf("ADC が無くても dry-run は通るはず: %v\n%s", bare.err, bare.stderr)
	}
}

func TestGmnADCはGeminiAPIでは読まない(t *testing.T) {
	tokens := newTokenStub(t)
	api, headers := apiStub(t)
	env := map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": writeServiceAccountKey(t, tokens.server.URL)}
	result := runGmn(t, t.TempDir(), env, "--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--api-key", "K1", "--server-url", api.URL, "generate", "hi", "--stream=false")
	if result.err != nil {
		t.Fatalf("generate failed: %v\n%s", result.err, result.stderr)
	}
	if got := headers(); len(got) != 1 || got[0].Get("x-goog-api-key") != "K1" || got[0].Get("Authorization") != "" {
		t.Errorf("headers = %v", got)
	}
	if n := tokens.callCount(); n != 0 {
		t.Errorf("トークンエンドポイントが %d 回呼ばれた", n)
	}
}

func TestGmnWhoamiが認証の出どころを表示する(t *testing.T) {
	cases := []struct {
		name   string
		env    map[string]string
		extra  []string
		source string
		value  string
	}{
		{"ADC", nil, nil, "adc", "ADC"},
		{"フラグ", nil, []string{"--access-token", "abcdefghijklmnop"}, "flag", "access-token"},
		{"環境変数", map[string]string{"GEMINI_ACCESS_TOKEN": "abcdefghijklmnop"}, nil, "env", "access-token"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := map[string]string{"GOOGLE_CLOUD_PROJECT": "p"}
			for k, v := range c.env {
				env[k] = v
			}
			base := append([]string{"--no-interactive", "--no-keyring", "--backend", "vertex"}, c.extra...)
			human := runGmn(t, t.TempDir(), env, append(base, "whoami")...)
			if human.err != nil {
				t.Fatalf("whoami failed: %v\n%s", human.err, human.stderr)
			}
			if !strings.Contains(human.stdout, c.value) {
				t.Errorf("whoami に %q が無い:\n%s", c.value, human.stdout)
			}
			machine := runGmn(t, t.TempDir(), env, append(base, "-o", "json", "whoami")...)
			var parsed struct {
				Backend map[string]map[string]any `json:"backend"`
			}
			if err := json.Unmarshal([]byte(machine.stdout), &parsed); err != nil {
				t.Fatalf("JSON ではない: %v\n%s", err, machine.stdout)
			}
			if parsed.Backend["auth"]["source"] != c.source {
				t.Errorf("auth = %v, want source %s", parsed.Backend["auth"], c.source)
			}
		})
	}
}

// whoamiADC は whoami -o json の backend.auth.adc（ADC をどこから読むか）を返す。
func whoamiADC(t *testing.T, home string, env map[string]string) (human string, adc map[string]any) {
	t.Helper()
	all := map[string]string{"GOOGLE_CLOUD_PROJECT": "p"}
	for k, v := range env {
		all[k] = v
	}
	base := []string{"--no-interactive", "--no-keyring", "--backend", "vertex"}
	h := runGmn(t, home, all, append(base, "whoami")...)
	if h.err != nil {
		t.Fatalf("whoami failed: %v\n%s", h.err, h.stderr)
	}
	m := runGmn(t, home, all, append(base, "-o", "json", "whoami")...)
	var parsed struct {
		Backend map[string]map[string]any `json:"backend"`
	}
	if err := json.Unmarshal([]byte(m.stdout), &parsed); err != nil {
		t.Fatalf("JSON ではない: %v\n%s", err, m.stdout)
	}
	adc, _ = parsed.Backend["auth"]["adc"].(map[string]any)
	if adc == nil {
		t.Fatalf("backend.auth.adc が無い: %s", m.stdout)
	}
	return h.stdout, adc
}

// writeGcloudADC は gcloud auth application-default login が書くファイルを HOME の下に置く。
func writeGcloudADC(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".config", "gcloud")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "application_default_credentials.json")
	body := `{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGmnWhoamiはGOOGLE_APPLICATION_CREDENTIALSのファイルと種類とアカウントを示す(t *testing.T) {
	home := t.TempDir()
	gcloud := writeGcloudADC(t, home)
	key := writeServiceAccountKey(t, "http://127.0.0.1:1/token")
	human, adc := whoamiADC(t, home, map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": key})

	for _, want := range []string{"GOOGLE_APPLICATION_CREDENTIALS", key, "service_account", "sa@stub-proj.iam.gserviceaccount.com", gcloud} {
		if !strings.Contains(human, want) {
			t.Errorf("whoami に %q が無い:\n%s", want, human)
		}
	}
	if adc["origin"] != "GOOGLE_APPLICATION_CREDENTIALS" || adc["file"] != key || adc["type"] != "service_account" ||
		adc["account"] != "sa@stub-proj.iam.gserviceaccount.com" || adc["ignored_file"] != gcloud {
		t.Errorf("adc = %v", adc)
	}
	if strings.Contains(human, "PRIVATE KEY") {
		t.Errorf("鍵の中身を出している:\n%s", human)
	}
}

func TestGmnWhoamiはgcloudのADCファイルを示す(t *testing.T) {
	home := t.TempDir()
	gcloud := writeGcloudADC(t, home)
	human, adc := whoamiADC(t, home, nil)

	if !strings.Contains(human, gcloud) || !strings.Contains(human, "authorized_user") {
		t.Errorf("whoami に gcloud の ADC ファイルが無い:\n%s", human)
	}
	if adc["origin"] != "gcloud" || adc["file"] != gcloud || adc["type"] != "authorized_user" {
		t.Errorf("adc = %v", adc)
	}
	if strings.Contains(human, "refresh_token") || strings.Contains(human, `"r"`) {
		t.Errorf("資格情報の中身を出している:\n%s", human)
	}
}

func TestGmnWhoamiはADCファイルが無ければメタデータサーバーを示す(t *testing.T) {
	human, adc := whoamiADC(t, t.TempDir(), nil)
	if !strings.Contains(human, "metadata server") {
		t.Errorf("whoami にメタデータサーバーが無い:\n%s", human)
	}
	if adc["origin"] != "metadata" {
		t.Errorf("adc = %v", adc)
	}
}

func TestGmnWhoamiはGOOGLE_APPLICATION_CREDENTIALSが読めないとそう示す(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	human, adc := whoamiADC(t, t.TempDir(), map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": missing})
	if !strings.Contains(human, missing) || !strings.Contains(human, "cannot read") {
		t.Errorf("whoami に読めない旨が無い:\n%s", human)
	}
	if adc["origin"] != "GOOGLE_APPLICATION_CREDENTIALS" || adc["file"] != missing || adc["error"] == nil {
		t.Errorf("adc = %v", adc)
	}
}
