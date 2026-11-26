package contract_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// gemini-api-plus の models list・get の契約。Agent Platform では公開モデルの一覧（publishers/google/models）に送り、
// 応答を Gemini API の models の形に写す。Gemini API のときは本家の挙動のまま。

// modelsStub は受け取ったリクエストを記録し、パスごとに応答を返すスタブ。
type modelsStub struct {
	mu       sync.Mutex
	requests []stubRequest
	server   *httptest.Server
}

type stubRequest struct {
	Path      string
	PageSize  string
	PageToken string
	Auth      string
	UserAgent string
	APIKey    string
}

func newModelsStub(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) *modelsStub {
	t.Helper()
	s := &modelsStub{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, stubRequest{
			Path:      r.URL.Path,
			PageSize:  r.URL.Query().Get("pageSize"),
			PageToken: r.URL.Query().Get("pageToken"),
			Auth:      r.Header.Get("Authorization"),
			UserAgent: r.Header.Get("User-Agent"),
			APIKey:    r.Header.Get("x-goog-api-key"),
		})
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		respond(w, r)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *modelsStub) got() []stubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubRequest(nil), s.requests...)
}

// publisherListBody は Agent Platform の publishers.models.list の応答を返す。
func publisherListBody(names ...string) string {
	items := make([]string, 0, len(names))
	for _, n := range names {
		items = append(items, fmt.Sprintf(`{"name":"publishers/google/models/%s","versionId":"001"}`, n))
	}
	return `{"publisherModels":[` + strings.Join(items, ",") + `]}`
}

// vertexArgs は Agent Platform 向けの共通フラグ。--server-url はスタブへ向けるために使う。
func vertexArgs(serverURL string, extra ...string) []string {
	base := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--access-token", "tok-123",
		"--server-url", serverURL}
	return append(base, extra...)
}

func TestPlusModelsAgentPlatformのlistは上限内のpageSizeで基盤モデルの一覧に送る(t *testing.T) {
	stub := newModelsStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, publisherListBody("gemini-2.5-flash"))
	})
	result := runPlus(t, t.TempDir(), nil, vertexArgs(stub.server.URL, "models", "list")...)
	if result.err != nil {
		t.Fatalf("models list failed: %v\n%s", result.err, result.stderr)
	}
	reqs := stub.got()
	if len(reqs) != 1 {
		t.Fatalf("リクエスト数 = %d, want 1", len(reqs))
	}
	if reqs[0].Path != "/v1beta1/publishers/google/models" {
		t.Errorf("path = %s, want /v1beta1/publishers/google/models", reqs[0].Path)
	}
	if reqs[0].PageSize != "100" {
		t.Errorf("pageSize = %q, want 100（Agent Platform の上限内の既定）", reqs[0].PageSize)
	}
	if reqs[0].Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", reqs[0].Auth)
	}
}

func TestPlusModelsAgentPlatformの上限超のpage_sizeは通信前にエラーで止まる(t *testing.T) {
	stub := newModelsStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, publisherListBody("x"))
	})
	result := runPlus(t, t.TempDir(), nil, vertexArgs(stub.server.URL, "models", "list", "--page-size", "101")...)
	if code := exitCodeOf(result.err); code != 2 {
		t.Fatalf("終了コード = %d, want 2\n%s", code, result.stderr)
	}
	if !strings.Contains(result.stderr, "100") {
		t.Errorf("stderr に上限 100 の説明が無い: %s", result.stderr)
	}
	if n := len(stub.got()); n != 0 {
		t.Errorf("通信前に止まるはずが %d 件送っている", n)
	}
}

func TestPlusModelsAgentPlatformのlistのjqでモデル名を取り出せる(t *testing.T) {
	stub := newModelsStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, publisherListBody("gemini-2.5-flash", "gemini-2.5-pro"))
	})
	result := runPlus(t, t.TempDir(), nil, vertexArgs(stub.server.URL, "models", "list", "--jq", "[.models[].name]")...)
	if result.err != nil {
		t.Fatalf("models list --jq failed: %v\n%s", result.err, result.stderr)
	}
	var names []string
	if err := json.Unmarshal([]byte(result.stdout), &names); err != nil {
		t.Fatalf("jq の出力が JSON 配列ではない: %v\n%s", err, result.stdout)
	}
	want := []string{"models/gemini-2.5-flash", "models/gemini-2.5-pro"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Errorf("names = %v, want %v", names, want)
	}
}

func TestPlusModelsAgentPlatformのgetはmodels接頭辞の有無のどちらでも同じ形で返る(t *testing.T) {
	stub := newModelsStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"publishers/google/models/gemini-2.5-flash","versionId":"001"}`)
	})
	for _, id := range []string{"gemini-2.5-flash", "models/gemini-2.5-flash"} {
		result := runPlus(t, t.TempDir(), nil, vertexArgs(stub.server.URL, "models", "get", id)...)
		if result.err != nil {
			t.Fatalf("models get %s failed: %v\n%s", id, result.err, result.stderr)
		}
		var got struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
			t.Fatalf("models get %s の出力が JSON ではない: %v\n%s", id, err, result.stdout)
		}
		if got.Name != "models/gemini-2.5-flash" {
			t.Errorf("models get %s: name = %q, want models/gemini-2.5-flash", id, got.Name)
		}
	}
	reqs := stub.got()
	if len(reqs) != 2 {
		t.Fatalf("リクエスト数 = %d, want 2", len(reqs))
	}
	for _, r := range reqs {
		if r.Path != "/v1beta1/publishers/google/models/gemini-2.5-flash" {
			t.Errorf("path = %s, want /v1beta1/publishers/google/models/gemini-2.5-flash", r.Path)
		}
	}
}

func TestPlusModelsAgentPlatformの_allは2ページ目まで辿る(t *testing.T) {
	stub := newModelsStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			fmt.Fprint(w, `{"publisherModels":[{"name":"publishers/google/models/page1-model"}],"nextPageToken":"tok-2"}`)
			return
		}
		fmt.Fprint(w, publisherListBody("page2-model"))
	})
	result := runPlus(t, t.TempDir(), nil, vertexArgs(stub.server.URL, "models", "list", "--all")...)
	if result.err != nil {
		t.Fatalf("models list --all failed: %v\n%s", result.err, result.stderr)
	}
	reqs := stub.got()
	if len(reqs) != 2 {
		t.Fatalf("リクエスト数 = %d, want 2", len(reqs))
	}
	if reqs[1].PageToken != "tok-2" {
		t.Errorf("2 ページ目の pageToken = %q, want tok-2", reqs[1].PageToken)
	}
	for _, want := range []string{"models/page1-model", "models/page2-model"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("出力に %s が無い: %s", want, result.stdout)
		}
	}
}

func TestPlusModelsAgentPlatformのdry_runはAgentPlatformのパスを示す(t *testing.T) {
	result := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--dry-run", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--access-token", "tok-123", "models", "list")
	if result.err != nil {
		t.Fatalf("models list --dry-run failed: %v\n%s", result.err, result.stderr)
	}
	var parsed struct {
		Request struct {
			URL string `json:"url"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("dry-run の出力が JSON ではない: %v\n%s", err, result.stdout)
	}
	want := "https://aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=100"
	if parsed.Request.URL != want {
		t.Errorf("url = %s, want %s", parsed.Request.URL, want)
	}
}

func TestPlusModelsGeminiAPIのlistは本家のまま1000件で送る(t *testing.T) {
	stub := newModelsStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"models":[{"name":"models/gemini-x"}]}`)
	})
	result := runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--api-key", "K1", "--server-url", stub.server.URL, "models", "list")
	if result.err != nil {
		t.Fatalf("models list failed: %v\n%s", result.err, result.stderr)
	}
	reqs := stub.got()
	if len(reqs) != 1 {
		t.Fatalf("リクエスト数 = %d, want 1", len(reqs))
	}
	if reqs[0].PageSize != "1000" {
		t.Errorf("pageSize = %q, want 1000（本家の既定）", reqs[0].PageSize)
	}
	if reqs[0].APIKey != "K1" {
		t.Errorf("x-goog-api-key = %q, want K1", reqs[0].APIKey)
	}
}

// #9 と #5 を並行で作ったため、models の経路だけ ADC を通らず「access token が要る」で止まっていた。
func TestPlusModelsAgentPlatformはADCのトークンで送る(t *testing.T) {
	tokens := newTokenStub(t)
	stub := newModelsStub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, publisherListBody("gemini-a"))
	})
	env := map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": writeServiceAccountKey(t, tokens.server.URL)}
	result := runPlus(t, t.TempDir(), env, "--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--server-url", stub.server.URL, "models", "list")
	if result.err != nil {
		t.Fatalf("models list failed: %v\n%s", result.err, result.stderr)
	}
	got := stub.got()
	if len(got) != 1 || got[0].Auth != "Bearer tok-1" {
		t.Fatalf("requests = %+v", got)
	}
}
