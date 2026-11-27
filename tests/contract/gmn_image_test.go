package contract_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Agent Platform の image（#11）の契約。Interactions API は image のモデルを拒否するので、
// generateContent に送る。Agent Platform には実際に送らず、スタブで確かめる。

// generateContentStub は generateContent の受け口。最後のリクエストを記録し、決めた応答を返す。
type generateContentStub struct {
	*httptest.Server
	mu       sync.Mutex
	path     string
	header   http.Header
	rawBody  []byte
	requests int
}

func newGenerateContentStub(t *testing.T, response string) *generateContentStub {
	t.Helper()
	s := &generateContentStub{}
	s.Server = httptest.NewServer(onlyFromCLI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.path, s.header, s.rawBody = r.URL.Path, r.Header.Clone(), raw
		s.requests++
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
	})))
	t.Cleanup(s.Close)
	return s
}

func (s *generateContentStub) lastPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path
}

func (s *generateContentStub) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

func (s *generateContentStub) lastHeader() http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.header
}

func (s *generateContentStub) body(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	raw := append([]byte(nil), s.rawBody...)
	s.mu.Unlock()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("リクエスト本文が JSON ではない: %v\n%s", err, raw)
	}
	return body
}

// parts は contents[0].parts を返す。
func (s *generateContentStub) parts(t *testing.T) []map[string]any {
	t.Helper()
	contents, _ := s.body(t)["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("contents = %v", contents)
	}
	first, _ := contents[0].(map[string]any)
	if first["role"] != "user" {
		t.Errorf("role = %v, want user", first["role"])
	}
	var out []map[string]any
	for _, p := range first["parts"].([]any) {
		out = append(out, p.(map[string]any))
	}
	return out
}

func (s *generateContentStub) imageConfig(t *testing.T) map[string]any {
	t.Helper()
	gc, _ := s.body(t)["generationConfig"].(map[string]any)
	cfg, _ := gc["imageConfig"].(map[string]any)
	return cfg
}

const (
	finalImage = "FINAL-IMAGE-BYTES"
	draftImage = "DRAFT-IMAGE-BYTES"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// thoughtMixedResponse は途中経過（thought のテキストと下書き画像）が先に付き、最後に最終画像が来る応答。
func thoughtMixedResponse(mime string) string {
	return fmt.Sprintf(`{"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[
		{"text":"考え中","thought":true},
		{"inlineData":{"mimeType":"image/png","data":%q},"thought":true},
		{"inlineData":{"mimeType":%q,"data":%q}}
	]}}]}`, b64(draftImage), mime, b64(finalImage))
}

func imageVertexArgs(stub *generateContentStub, extra ...string) []string {
	args := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--server-url", stub.URL, "--access-token", "tok", "image"}
	return append(args, extra...)
}

// writtenPath は -o json の出力から書き出したファイルのパスを取る。
func writtenPath(t *testing.T, stdout string) string {
	t.Helper()
	var env struct {
		Path     string `json:"path"`
		Kind     string `json:"kind"`
		MimeType string `json:"mime_type"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout が JSON ではない: %v\n%s", err, stdout)
	}
	if env.Path == "" || env.Kind != "image" {
		t.Fatalf("envelope = %s", stdout)
	}
	return env.Path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestGmnImage_AgentPlatformはgenerateContentに既定モデルで送る(t *testing.T) {
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/png"))
	out := filepath.Join(t.TempDir(), "cat.png")
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "a cat", "--out", out)...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	if want := "/v1/projects/my-proj/locations/global/publishers/google/models/gemini-nano-banana-2.1:generateContent"; stub.lastPath() != want {
		t.Errorf("path = %s, want %s", stub.lastPath(), want)
	}
	want := `{"contents":[{"role":"user","parts":[{"text":"a cat"}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"]}}`
	var wantBody map[string]any
	if err := json.Unmarshal([]byte(want), &wantBody); err != nil {
		t.Fatal(err)
	}
	if got := stub.body(t); !reflect.DeepEqual(got, wantBody) {
		t.Errorf("本文 = %v\nwant %v", got, wantBody)
	}
	if h := stub.lastHeader(); h.Get("Authorization") != "Bearer tok" || h.Get("X-Goog-User-Project") != "my-proj" {
		t.Errorf("ヘッダー = %v", h)
	}
}

func TestGmnImage_thoughtの画像は書かず最終画像だけを書き出す(t *testing.T) {
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/png"))
	out := filepath.Join(t.TempDir(), "cat.png")
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "a cat", "--out", out)...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	if got := readFile(t, out); got != finalImage {
		t.Errorf("書き出した中身 = %q, want %q", got, finalImage)
	}
}

func TestGmnImage_最終画像が先頭でもthoughtを避ける(t *testing.T) {
	// 3.1-flash-image のように最終画像が parts[0] に来て、後ろに thought が付く並び。
	resp := fmt.Sprintf(`{"candidates":[{"content":{"parts":[
		{"inlineData":{"mimeType":"image/png","data":%q}},
		{"inlineData":{"mimeType":"image/png","data":%q},"thought":true}
	]}}]}`, b64(finalImage), b64(draftImage))
	stub := newGenerateContentStub(t, resp)
	out := filepath.Join(t.TempDir(), "a.png")
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "x", "--out", out)...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	if got := readFile(t, out); got != finalImage {
		t.Errorf("書き出した中身 = %q, want %q", got, finalImage)
	}
}

func TestGmnImage_outのファイルと既定のファイル名とstdoutのパス(t *testing.T) {
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/jpeg"))

	dir := t.TempDir()
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "a cat", "--out", dir)...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	path := writtenPath(t, result.stdout)
	if filepath.Dir(path) != dir || !strings.HasPrefix(filepath.Base(path), "gemini-image-") || filepath.Ext(path) != ".jpg" {
		t.Errorf("既定のファイル名 = %s, want %s/gemini-image-*.jpg（mimeType 由来の拡張子）", path, dir)
	}
	if got := readFile(t, path); got != finalImage {
		t.Errorf("書き出した中身 = %q", got)
	}

	file := filepath.Join(t.TempDir(), "hero.jpg")
	result = runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "a cat", "--out", file)...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	if got := writtenPath(t, result.stdout); got != file {
		t.Errorf("path = %s, want %s", got, file)
	}

	// 人間向けの出力（-o pretty）は stdout にパスだけを出す。
	pretty := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "pretty",
		"--backend", "vertex", "--project", "my-proj", "--server-url", stub.URL, "--access-token", "tok", "image", "a cat", "--out", file}
	result = runGmn(t, t.TempDir(), nil, pretty...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	if strings.TrimSpace(result.stdout) != file {
		t.Errorf("stdout = %q, want パス %s だけ", result.stdout, file)
	}
}

func TestGmnImage_modelで送り先が変わりunsupportedは通信前に候補つきで止まる(t *testing.T) {
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/png"))
	out := filepath.Join(t.TempDir(), "a.png")
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "a cat", "--model", "gemini-3-pro-image", "--out", out)...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	if want := "/v1/projects/my-proj/locations/global/publishers/google/models/gemini-3-pro-image:generateContent"; stub.lastPath() != want {
		t.Errorf("path = %s, want %s", stub.lastPath(), want)
	}

	before := stub.requestCount()
	result = runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "a cat", "--model", "nano-banana-pro-preview", "--out", out)...)
	if exitCodeOf(result.err) != 2 {
		t.Fatalf("exit = %d, want 2\n%s", exitCodeOf(result.err), result.stderr)
	}
	if !strings.Contains(result.stderr, "gemini-3-pro-image") {
		t.Errorf("エラーに候補が無い:\n%s", result.stderr)
	}
	if stub.requestCount() != before {
		t.Error("止めるべきなのに送信した")
	}
}

func TestGmnImage_aspect_ratioとimage_sizeとoutput_mime_typeをimageConfigに入れる(t *testing.T) {
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/png"))
	out := filepath.Join(t.TempDir(), "a.png")
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "a cat",
		"--aspect-ratio", "16:9", "--image-size", "2K", "--output-mime-type", "image/jpeg", "--out", out)...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	cfg := stub.imageConfig(t)
	if cfg["aspectRatio"] != "16:9" || cfg["imageSize"] != "2K" {
		t.Errorf("imageConfig = %v", cfg)
	}
	opts, _ := cfg["imageOutputOptions"].(map[string]any)
	if opts["mimeType"] != "image/jpeg" {
		t.Errorf("imageOutputOptions = %v", opts)
	}
	modalities, _ := stub.body(t)["generationConfig"].(map[string]any)["responseModalities"].([]any)
	if len(modalities) != 2 {
		t.Errorf("responseModalities が消えた: %v", modalities)
	}
}

func writeTestPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "in.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGmnImage_入力画像をinlineDataで画像の後にプロンプトを置く(t *testing.T) {
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/png"))
	in := writeTestPNG(t)
	out := filepath.Join(t.TempDir(), "a.png")
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "make it blue", "--input", in, "--out", out)...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	parts := stub.parts(t)
	if len(parts) != 2 {
		t.Fatalf("parts = %v", parts)
	}
	inline, _ := parts[0]["inlineData"].(map[string]any)
	raw, _ := os.ReadFile(in)
	if inline["mimeType"] != "image/png" || inline["data"] != base64.StdEncoding.EncodeToString(raw) {
		t.Errorf("parts[0] = %v", parts[0])
	}
	if parts[1]["text"] != "make it blue" {
		t.Errorf("parts[1] = %v", parts[1])
	}
}

func TestGmnImage_gs_URIの入力はfileDataで送る(t *testing.T) {
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/png"))
	out := filepath.Join(t.TempDir(), "a.png")
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "make it blue", "--input", "gs://bucket/in.jpg", "--out", out)...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	parts := stub.parts(t)
	fileData, _ := parts[0]["fileData"].(map[string]any)
	if len(parts) != 2 || fileData["fileUri"] != "gs://bucket/in.jpg" || fileData["mimeType"] != "image/jpeg" {
		t.Errorf("parts = %v", parts)
	}
}

func TestGmnImage_previous_interaction_idはAgentPlatformで分かるエラーにする(t *testing.T) {
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/png"))
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "make it blue", "--previous-interaction-id", "v1_abc")...)
	if exitCodeOf(result.err) != 2 {
		t.Fatalf("exit = %d, want 2\n%s", exitCodeOf(result.err), result.stderr)
	}
	for _, want := range []string{"--previous-interaction-id", "Agent Platform", "--input"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("エラーに %q が無い:\n%s", want, result.stderr)
		}
	}
	if stub.requestCount() != 0 {
		t.Error("止めるべきなのに送信した")
	}
}

func TestGmnImage_画像が無い応答はテキストとfinishReasonを含むエラーにする(t *testing.T) {
	resp := `{"candidates":[{"finishReason":"IMAGE_SAFETY","content":{"parts":[
		{"text":"下書き","thought":true},{"text":"お断りします"}]}}]}`
	stub := newGenerateContentStub(t, resp)
	out := filepath.Join(t.TempDir(), "a.png")
	result := runGmn(t, t.TempDir(), nil, imageVertexArgs(stub, "x", "--out", out)...)
	if result.err == nil {
		t.Fatal("画像が無いのに成功した")
	}
	for _, want := range []string{"IMAGE_SAFETY", "お断りします"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("エラーに %q が無い:\n%s", want, result.stderr)
		}
	}
	if strings.Contains(result.stderr, "下書き") {
		t.Errorf("thought のテキストをエラーに出している:\n%s", result.stderr)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("画像が無いのにファイルを書いた")
	}
}

func TestGmnImage_dry_runは送らずgenerateContentの宛先と本文を示す(t *testing.T) {
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/png"))
	args := append([]string{"--dry-run"}, imageVertexArgs(stub, "a cat")...)
	result := runGmn(t, t.TempDir(), nil, args...)
	if result.err != nil {
		t.Fatalf("image --dry-run failed: %v\n%s", result.err, result.stderr)
	}
	if stub.requestCount() != 0 {
		t.Error("--dry-run なのに送信した")
	}
	for _, want := range []string{":generateContent", "responseModalities", "[REDACTED]"} {
		if !strings.Contains(result.stdout+result.stderr, want) {
			t.Errorf("--dry-run の出力に %q が無い:\n%s\n%s", want, result.stdout, result.stderr)
		}
	}
	if strings.Contains(result.stdout+result.stderr, "Bearer tok") {
		t.Error("--dry-run にトークンが出ている")
	}
}

func TestGmnImage_ADCのトークンで認証する(t *testing.T) {
	tokens := newTokenStub(t)
	stub := newGenerateContentStub(t, thoughtMixedResponse("image/png"))
	env := map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": writeServiceAccountKey(t, tokens.server.URL)}
	out := filepath.Join(t.TempDir(), "a.png")
	args := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--server-url", stub.URL, "image", "a cat", "--out", out}
	result := runGmn(t, t.TempDir(), env, args...)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	if got := stub.lastHeader().Get("Authorization"); got != "Bearer tok-1" {
		t.Errorf("Authorization = %q, want Bearer tok-1", got)
	}
	if strings.Contains(result.stdout+result.stderr, "tok-1") {
		t.Errorf("出力にトークンが漏れている:\n%s\n%s", result.stdout, result.stderr)
	}
}

func TestGmnImage_GeminiAPIのときは本家のままInteractionsに送る(t *testing.T) {
	resp := fmt.Sprintf(`{"id":"i","object":"interaction","status":"completed","steps":[{"type":"model_output","content":[{"type":"image","mime_type":"image/png","data":%q}]}]}`, b64(finalImage))
	stub := newInteractionStub(t, resp)
	out := filepath.Join(t.TempDir(), "a.png")
	result := runGmn(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--server-url", stub.URL, "--api-key", "k", "image", "a cat", "--out", out)
	if result.err != nil {
		t.Fatalf("image failed: %v\n%s", result.err, result.stderr)
	}
	if stub.requests.Load() != 1 || stub.requestBody(t)["model"] != "gemini-nano-banana-2.1" {
		t.Errorf("Interactions に送られていない: %v", stub.requestBody(t))
	}
	if got := readFile(t, out); got != finalImage {
		t.Errorf("書き出した中身 = %q", got)
	}
}
