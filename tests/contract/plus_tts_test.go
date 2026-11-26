package contract_test

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Agent Platform の tts（#12）の契約。Interactions API は tts のモデルを通さないので generateContent に送る。
// Agent Platform には実際に送らず、スタブで確かめる。

// ttsResponse は音声 1 件だけの generateContent 応答。
func ttsResponse(mime string, audio []byte) string {
	return fmt.Sprintf(`{"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[
		{"inlineData":{"mimeType":%q,"data":%q}}
	]}}]}`, mime, b64(string(audio)))
}

func ttsVertexArgs(stub *generateContentStub, extra ...string) []string {
	args := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--server-url", stub.URL, "--access-token", "tok", "tts", "Hello from Agent Platform"}
	return append(args, extra...)
}

// parseWAV は 44 バイトの WAV ヘッダの rate・channels と、続く PCM を返す。
func parseWAV(t *testing.T, data []byte) (rate, channels uint32, pcm []byte) {
	t.Helper()
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("WAV ではない: 先頭 %q", data[:min(len(data), 12)])
	}
	channels = uint32(binary.LittleEndian.Uint16(data[22:24]))
	rate = binary.LittleEndian.Uint32(data[24:28])
	return rate, channels, data[44:]
}

func TestPlusTTS_AgentPlatformはgenerateContentに既定モデルと声を送る(t *testing.T) {
	stub := newGenerateContentStub(t, ttsResponse("audio/wav", []byte("RIFF-WAV-BYTES")))
	out := filepath.Join(t.TempDir(), "hello.wav")
	result := runPlus(t, t.TempDir(), nil, ttsVertexArgs(stub, "--out", out)...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	if want := "/v1/projects/my-proj/locations/global/publishers/google/models/gemini-3.8-flash-tts:generateContent"; stub.lastPath() != want {
		t.Errorf("path = %s, want %s", stub.lastPath(), want)
	}
	want := `{"contents":[{"role":"user","parts":[{"text":"Hello from Agent Platform"}]}],"generationConfig":{"responseModalities":["AUDIO"],"speechConfig":{"voiceConfig":{"prebuiltVoiceConfig":{"voiceName":"Kore"}}}}}`
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

func TestPlusTTS_audio_wavの応答はバイトそのままを書き出す(t *testing.T) {
	wav := append([]byte("RIFF"), []byte{0, 1, 2, 3, 0xff, 0xfe}...)
	stub := newGenerateContentStub(t, ttsResponse("audio/wav", wav))
	out := filepath.Join(t.TempDir(), "a.wav")
	result := runPlus(t, t.TempDir(), nil, ttsVertexArgs(stub, "--out", out)...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	if got := readFile(t, out); got != string(wav) {
		t.Errorf("書き出した中身 = %q, want %q", got, wav)
	}
}

func TestPlusTTS_生PCMのl16はWAVに包みrateとchannelsを入れる(t *testing.T) {
	pcm := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	stub := newGenerateContentStub(t, ttsResponse("audio/l16; rate=24000; channels=1", pcm))
	out := filepath.Join(t.TempDir(), "a.wav")
	result := runPlus(t, t.TempDir(), nil, ttsVertexArgs(stub, "--out", out)...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	rate, channels, got := parseWAV(t, []byte(readFile(t, out)))
	if rate != 24000 || channels != 1 {
		t.Errorf("rate/channels = %d/%d, want 24000/1", rate, channels)
	}
	if string(got) != string(pcm) {
		t.Errorf("PCM = %v, want %v", got, pcm)
	}
}

func TestPlusTTS_codec付きのL16はrateとchannelsを応答から読む(t *testing.T) {
	// channels が無い書式は mono、rate は応答の値を使う（既定の 24000 に落とさない）。
	stub := newGenerateContentStub(t, ttsResponse("audio/L16;codec=pcm;rate=48000", []byte{9, 9}))
	out := filepath.Join(t.TempDir(), "b.wav")
	result := runPlus(t, t.TempDir(), nil, ttsVertexArgs(stub, "--out", out)...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	rate, channels, _ := parseWAV(t, []byte(readFile(t, out)))
	if rate != 48000 || channels != 1 {
		t.Errorf("rate/channels = %d/%d, want 48000/1", rate, channels)
	}
}

func TestPlusTTS_voiceはvoiceNameに入る(t *testing.T) {
	stub := newGenerateContentStub(t, ttsResponse("audio/wav", []byte("RIFF")))
	out := filepath.Join(t.TempDir(), "p.wav")
	result := runPlus(t, t.TempDir(), nil, ttsVertexArgs(stub, "--voice", "Puck", "--out", out)...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	sc, _ := stub.body(t)["generationConfig"].(map[string]any)["speechConfig"].(map[string]any)
	name := sc["voiceConfig"].(map[string]any)["prebuiltVoiceConfig"].(map[string]any)["voiceName"]
	if name != "Puck" {
		t.Errorf("voiceName = %v, want Puck", name)
	}
}

func TestPlusTTS_styleはテキストへの指示として前置しspeechConfigには入れない(t *testing.T) {
	// Agent Platform の SpeechConfig に style のキーは無い（GenerationConfig リファレンス）。
	stub := newGenerateContentStub(t, ttsResponse("audio/wav", []byte("RIFF")))
	out := filepath.Join(t.TempDir(), "s.wav")
	result := runPlus(t, t.TempDir(), nil, ttsVertexArgs(stub, "--style", "whispered urgently", "--out", out)...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	parts := stub.parts(t)
	if got, want := parts[0]["text"], "Say whispered urgently: Hello from Agent Platform"; got != want {
		t.Errorf("text = %v, want %q", got, want)
	}
	if strings.Contains(stub.body(t)["generationConfig"].(map[string]any)["speechConfig"].(map[string]any)["voiceConfig"].(map[string]any)["prebuiltVoiceConfig"].(map[string]any)["voiceName"].(string), "whispered") {
		t.Errorf("style が voiceName に混ざっている")
	}
}

func TestPlusTTS_multi_speakerはmultiSpeakerVoiceConfigに写す(t *testing.T) {
	stub := newGenerateContentStub(t, ttsResponse("audio/wav", []byte("RIFF")))
	out := filepath.Join(t.TempDir(), "m.wav")
	// 複数話者は話者の名前で始まる台本が要る（本家の入力検査）。
	args := ttsVertexArgs(stub, "--multi-speaker", "Alice=Kore,Bob=Puck", "--out", out)
	for i := range args {
		if args[i] == "Hello from Agent Platform" {
			args[i] = "Alice: hi! Bob: hello."
		}
	}
	result := runPlus(t, t.TempDir(), nil, args...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	sc := stub.body(t)["generationConfig"].(map[string]any)["speechConfig"].(map[string]any)
	if _, ok := sc["voiceConfig"]; ok {
		t.Errorf("複数話者なのに voiceConfig が入っている: %v", sc)
	}
	want := []any{
		map[string]any{"speaker": "Alice", "voiceConfig": map[string]any{"prebuiltVoiceConfig": map[string]any{"voiceName": "Kore"}}},
		map[string]any{"speaker": "Bob", "voiceConfig": map[string]any{"prebuiltVoiceConfig": map[string]any{"voiceName": "Puck"}}},
	}
	got := sc["multiSpeakerVoiceConfig"].(map[string]any)["speakerVoiceConfigs"]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("speakerVoiceConfigs = %v\nwant %v", got, want)
	}
}

func TestPlusTTS_outのファイルと既定のファイル名とstdoutのパスとjson(t *testing.T) {
	stub := newGenerateContentStub(t, ttsResponse("audio/wav", []byte("RIFF-DEFAULT")))
	dir := t.TempDir()
	result := runPlus(t, t.TempDir(), nil, ttsVertexArgs(stub, "--out", dir)...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	var env struct {
		Path      string `json:"path"`
		MimeType  string `json:"mime_type"`
		SizeBytes int    `json:"size_bytes"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &env); err != nil {
		t.Fatalf("stdout が JSON ではない: %v\n%s", err, result.stdout)
	}
	if got := filepath.Dir(env.Path); got != dir {
		t.Errorf("書き出し先のディレクトリ = %s, want %s", got, dir)
	}
	if !regexp.MustCompile(`^gemini-tts-\d+-[0-9a-f]{6}\.wav$`).MatchString(filepath.Base(env.Path)) {
		t.Errorf("既定のファイル名 = %s", filepath.Base(env.Path))
	}
	if env.MimeType != "audio/wav" || env.SizeBytes != len("RIFF-DEFAULT") {
		t.Errorf("envelope = %s", result.stdout)
	}
	if got := readFile(t, env.Path); got != "RIFF-DEFAULT" {
		t.Errorf("書き出した中身 = %q", got)
	}
}

func TestPlusTTS_stdoutにはパスだけを出す(t *testing.T) {
	stub := newGenerateContentStub(t, ttsResponse("audio/wav", []byte("RIFF")))
	out := filepath.Join(t.TempDir(), "path.wav")
	args := ttsVertexArgs(stub, "--out", out)
	// -o json を外して既定の出力（パス）を見る。
	var plain []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" {
			i++
			continue
		}
		plain = append(plain, args[i])
	}
	result := runPlus(t, t.TempDir(), nil, plain...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	if got := strings.TrimSpace(result.stdout); got != out {
		t.Errorf("stdout = %q, want %q", got, out)
	}
}

func TestPlusTTS_音声が無い応答はテキストとfinishReasonを含むエラーにする(t *testing.T) {
	resp := `{"candidates":[{"finishReason":"SAFETY","content":{"role":"model","parts":[{"text":"できません"}]}}]}`
	stub := newGenerateContentStub(t, resp)
	out := filepath.Join(t.TempDir(), "none.wav")
	result := runPlus(t, t.TempDir(), nil, ttsVertexArgs(stub, "--out", out)...)
	if result.err == nil {
		t.Fatalf("音声が無いのに成功した: %s", result.stdout)
	}
	msg := result.stdout + result.stderr
	for _, want := range []string{"finishReason SAFETY", "できません"} {
		if !strings.Contains(msg, want) {
			t.Errorf("エラーに %q が無い:\n%s", want, msg)
		}
	}
}

func TestPlusTTS_voice_promptはAgentPlatformでは通信前に止まる(t *testing.T) {
	stub := newGenerateContentStub(t, ttsResponse("audio/wav", []byte("RIFF")))
	result := runPlus(t, t.TempDir(), nil, ttsVertexArgs(stub, "--voice-prompt", "Warm radio host")...)
	if result.err == nil {
		t.Fatalf("--voice-prompt が Agent Platform で成功した")
	}
	if n := stub.requestCount(); n != 0 {
		t.Errorf("通信した回数 = %d, want 0", n)
	}
	if !strings.Contains(result.stdout+result.stderr, "Agent Platform") {
		t.Errorf("Agent Platform で使えないと分かる文言が無い:\n%s%s", result.stdout, result.stderr)
	}
}

func TestPlusTTS_ADCのトークンで認証する(t *testing.T) {
	tokens := newTokenStub(t)
	stub := newGenerateContentStub(t, ttsResponse("audio/wav", []byte("RIFF")))
	env := map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": writeServiceAccountKey(t, tokens.server.URL)}
	out := filepath.Join(t.TempDir(), "adc.wav")
	args := []string{"--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--backend", "vertex", "--project", "my-proj", "--server-url", stub.URL, "tts", "Hello", "--out", out}
	result := runPlus(t, t.TempDir(), env, args...)
	if result.err != nil {
		t.Fatalf("tts failed: %v\n%s", result.err, result.stderr)
	}
	if got := stub.lastHeader().Get("Authorization"); got != "Bearer tok-1" {
		t.Errorf("Authorization = %q, want Bearer tok-1", got)
	}
	if strings.Contains(result.stdout+result.stderr, "tok-1") {
		t.Errorf("出力にトークンが漏れている:\n%s\n%s", result.stdout, result.stderr)
	}
}

func TestPlusTTS_GeminiAPIのときは本家のままInteractionsに送る(t *testing.T) {
	stub := newInteractionStub(t, `{"id":"i","object":"interaction","status":"completed","steps":[]}`)
	out := filepath.Join(t.TempDir(), "g.wav")
	runPlus(t, t.TempDir(), nil, "--no-interactive", "--no-keyring", "--no-retries", "-o", "json",
		"--server-url", stub.URL, "--api-key", "k", "tts", "Hello", "--out", out)
	if stub.requests.Load() != 1 || stub.requestBody(t)["model"] != "gemini-3.8-flash-tts" {
		t.Errorf("Interactions に送られていない: %v", stub.requestBody(t))
	}
}
