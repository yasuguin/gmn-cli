package contract_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Agent Platform の music の契約。Interactions API は Lyria に response_format（type: audio）を付けると
// 400（bit_rate must be positive・Audio bit_rate is not supported）で拒否し、既定の lyria-3.5 も受け付けない。
// response_format を付けずに送ると audio/mpeg を返す（実測 2026-11-27、global）。

// musicOKResponse は Agent Platform の Lyria の応答の形（steps が無く outputs に音声が入る）。"aGk=" は "hi"。
const musicOKResponse = `{"object":"interaction","status":"completed","model":"lyria-3-pro-preview","outputs":[{"type":"text","text":"<instrumental>"},{"type":"audio","mime_type":"audio/mpeg","data":"aGk="}]}`

// musicStub は Agent Platform の Interactions API の作成（/v1beta1/projects/<p>/locations/<l>/interactions）を受ける。
type musicStub struct {
	*httptest.Server
	mu       sync.Mutex
	path     string
	rawBody  []byte
	requests atomic.Int32
}

func newMusicStub(t *testing.T, response string) *musicStub {
	t.Helper()
	stub := &musicStub{}
	stub.Server = httptest.NewServer(onlyFromCLI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		stub.mu.Lock()
		stub.path, stub.rawBody = r.URL.Path, raw
		stub.mu.Unlock()
		stub.requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
	})))
	t.Cleanup(stub.Close)
	return stub
}

func (s *musicStub) requestBody(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var body map[string]any
	if err := json.Unmarshal(s.rawBody, &body); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, s.rawBody)
	}
	return body
}

func musicVertexArgs(stub *musicStub, extra ...string) []string {
	args := []string{"--no-interactive", "--no-keyring", "--no-retries",
		"--backend", "vertex", "--project", "my-proj", "--server-url", stub.URL, "--access-token", "tok", "music"}
	return append(args, extra...)
}

func TestGmnMusic_AgentPlatformの既定はresponse_formatを付けず代わりのモデルで送る(t *testing.T) {
	stub := newMusicStub(t, musicOKResponse)
	out := filepath.Join(t.TempDir(), "m.mp3")
	result := runGmn(t, t.TempDir(), nil, musicVertexArgs(stub, "calm piano", "--out", out)...)
	if result.err != nil {
		t.Fatalf("music failed: %v\n%s", result.err, result.stderr)
	}
	if want := "/v1beta1/projects/my-proj/locations/global/interactions"; stub.path != want {
		t.Errorf("path = %q, want %q", stub.path, want)
	}
	if got, err := os.ReadFile(out); err != nil || string(got) != "hi" {
		t.Errorf("書き出した音声 = %q, %v; want \"hi\"", got, err)
	}
	body := stub.requestBody(t)
	if got := body["model"]; got != "lyria-3-pro-preview" {
		t.Errorf("model = %v, want lyria-3-pro-preview", got)
	}
	if rf, ok := body["response_format"]; ok {
		t.Errorf("response_format を送っている: %v", rf)
	}
	want := "Using lyria-3-pro-preview: lyria-3.5 is not available on Agent Platform"
	if !strings.Contains(result.stderr, want) {
		t.Errorf("stderr に %q が無い:\n%s", want, result.stderr)
	}
}

func TestGmnMusic_AgentPlatformで明示したモデルはそのまま送りresponse_formatだけ外す(t *testing.T) {
	stub := newMusicStub(t, musicOKResponse)
	out := filepath.Join(t.TempDir(), "m.mp3")
	result := runGmn(t, t.TempDir(), nil, musicVertexArgs(stub, "calm piano", "-m", "lyria-3-clip-preview", "--out", out)...)
	if result.err != nil {
		t.Fatalf("music failed: %v\n%s", result.err, result.stderr)
	}
	body := stub.requestBody(t)
	if got := body["model"]; got != "lyria-3-clip-preview" {
		t.Errorf("model = %v, want lyria-3-clip-preview", got)
	}
	if rf, ok := body["response_format"]; ok {
		t.Errorf("response_format を送っている: %v", rf)
	}
}

func TestGmnMusic_AgentPlatformでlyria35を明示すると通信せず候補を示して終了コード2(t *testing.T) {
	stub := newMusicStub(t, musicOKResponse)
	result := runGmn(t, t.TempDir(), nil, musicVertexArgs(stub, "calm piano", "-m", "lyria-3.5", "--out", filepath.Join(t.TempDir(), "m.mp3"))...)
	if exitCodeOf(result.err) != 2 {
		t.Fatalf("exit = %d, want 2\n%s", exitCodeOf(result.err), result.stderr)
	}
	if !strings.Contains(result.stderr, "lyria-3-pro-preview") {
		t.Errorf("stderr に代わりの候補が無い:\n%s", result.stderr)
	}
	if n := stub.requests.Load(); n != 0 {
		t.Errorf("通信している: %d 回", n)
	}
}
