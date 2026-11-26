package custom

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google-gemini/gemini-api-cli/internal/clierrors"
	"github.com/google-gemini/gemini-api-cli/internal/output"
	"github.com/google-gemini/gemini-api-cli/internal/plus"
	"github.com/google-gemini/gemini-api-cli/internal/plus/backend"
	"github.com/google-gemini/gemini-api-cli/internal/plus/capability"
	"github.com/google-gemini/gemini-api-cli/internal/plus/gencontent"
	"github.com/google-gemini/gemini-api-cli/internal/plus/modelmap"
	"github.com/google-gemini/gemini-api-cli/internal/sdk/models/interactions"
	"github.com/spf13/cobra"
)

// ttsUsesGenerateContent は Agent Platform で tts を generateContent に送るときに真。
// Interactions API は tts のモデルを通さない（#12 の実測）。表に無いモデルも送る（image と同じ）。
func ttsUsesGenerateContent(cmd *cobra.Command, model string) bool {
	if !plus.Enabled() {
		return false
	}
	if b, err := backend.Resolve(cmd); err != nil || !b.IsVertex() {
		return false
	}
	if e, _ := capability.Lookup(capability.PathOf(cmd)); e.Kind != capability.GenerateContent {
		return false
	}
	st := modelmap.Lookup(model).Status
	return st == modelmap.GenerateContent || st == modelmap.Unverified
}

// runTTSGenerateContent は tts を generateContent で送り、返った音声を本家と同じ書き出しに載せる。
func runTTSGenerateContent(cmd *cobra.Command, model, text, style string, speech *interactions.SpeechConfigUnion, absent []string, voicePrompt string) error {
	// カスタムの声は voices API で作るもので、Agent Platform の generateContent には無い。
	if voicePrompt != "" {
		return usageError("--voice-prompt is not available on Agent Platform",
			"Use --voice with a prebuilt voice name (e.g. Kore, Puck)")
	}
	if name := customVoiceName(speech); name != "" {
		return usageError(fmt.Sprintf("custom voice %q is only available on the Gemini API", name),
			"Use a prebuilt voice name (e.g. Kore, Puck), or switch to the Gemini API with --backend gemini")
	}
	var warnings []string
	for _, speaker := range absent {
		warnings = append(warnings, fmt.Sprintf("speaker %q does not appear in the text", speaker))
	}
	for _, warning := range warnings {
		progress(cmd, "warning: %s", warning)
	}

	progress(cmd, "Synthesizing speech with %s...", model)
	stop := output.StartRequestProgress(cmd)
	res, err := gencontent.Call(cmd.Context(), cmd, model, ttsGenerateContentBody(text, style, speech))
	stop()
	if err != nil {
		return output.Error(cmd, err)
	}
	if res == nil { // --dry-run
		return nil
	}
	audio, mimeType, err := finalAudio(res)
	if err != nil {
		return output.Error(cmd, err)
	}
	path, envelope, err := writeGenerateContentAudio(cmd, model, audio, mimeType)
	if err != nil {
		return err
	}
	if len(warnings) > 0 {
		envelope["warnings"] = warnings
	}
	return emitResult(cmd, path, envelope)
}

// 本家 tts.go の書き出し部を共有せずに写している（本家ファイルへ足すのは呼び出し行だけにするため）。
func writeGenerateContentAudio(cmd *cobra.Command, model string, audio []byte, mimeType string) (string, map[string]any, error) {
	format := parseAudioMIME(mimeType)
	channels, sampleRate := format.channels, format.sampleRate
	data := audio
	if format.isPCM {
		if channels == 0 {
			channels = 1
		}
		if sampleRate == 0 {
			sampleRate = 24000
		}
		data = append(wavHeader(len(audio), sampleRate, channels), audio...)
	}
	ext, ok := extensionForAudioMIME(mimeType)
	if !ok {
		return "", nil, runtimeError(fmt.Sprintf("the API returned audio as %s, which cannot be written as a playable file", mimeType))
	}
	path, err := artifactPath(cmd, "out", "gemini-tts", ext)
	if err != nil {
		return "", nil, err
	}
	if err := writeArtifact(path, data); err != nil {
		return "", nil, runtimeError(fmt.Sprintf("cannot write %s: %v", path, err))
	}
	progress(cmd, "Wrote %s (%d bytes).", path, len(data))
	envelope := map[string]any{
		"model":      model,
		"path":       path,
		"mime_type":  mimeType,
		"size_bytes": len(data),
	}
	if format.isPCM {
		envelope["sample_rate"] = sampleRate
		envelope["channels"] = channels
	}
	return path, envelope, nil
}

// customVoiceName は speech の中の voices API で作ったカスタム音声（voice_ で始まる ID）を返す。無ければ空。
func customVoiceName(speech *interactions.SpeechConfigUnion) string {
	var voices []*string
	if speech.SpeakerConfig == nil {
		for _, v := range speech.ArrayOfSpeechConfig {
			voices = append(voices, v.Voice)
		}
	} else {
		for _, sp := range speech.SpeakerConfig.Speakers {
			voices = append(voices, sp.Voice)
		}
	}
	for _, v := range voices {
		if v != nil && strings.HasPrefix(*v, "voice_") {
			return *v
		}
	}
	return ""
}

// ttsGenerateContentBody は tts の入力と声の設定を generateContent の本文にする。
// style は SpeechConfig に対応するキーが無いので、読み上げの指示としてテキストの前に置く。
func ttsGenerateContentBody(text, style string, speech *interactions.SpeechConfigUnion) map[string]any {
	if style = strings.TrimSpace(style); style != "" {
		text = "Say " + style + ": " + text
	}
	sc := map[string]any{}
	if speech.SpeakerConfig == nil {
		v := speech.ArrayOfSpeechConfig[0]
		sc["voiceConfig"] = prebuiltVoiceConfig(*v.Voice)
		if v.Language != nil {
			sc["languageCode"] = *v.Language
		}
	} else {
		var cfgs []any
		for _, sp := range speech.SpeakerConfig.Speakers {
			cfgs = append(cfgs, map[string]any{
				"speaker":     *sp.Speaker,
				"voiceConfig": prebuiltVoiceConfig(*sp.Voice),
			})
			if _, ok := sc["languageCode"]; !ok && sp.Language != nil {
				sc["languageCode"] = *sp.Language
			}
		}
		sc["multiSpeakerVoiceConfig"] = map[string]any{"speakerVoiceConfigs": cfgs}
	}
	return map[string]any{
		"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": text}}}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"speechConfig":       sc,
		},
	}
}

func prebuiltVoiceConfig(voice string) map[string]any {
	return map[string]any{"prebuiltVoiceConfig": map[string]any{"voiceName": voice}}
}

// finalAudio は応答の最後の thought でない音声を返す。無ければ、応答のテキストと終了理由を含むエラーにする。
func finalAudio(res *gencontent.Response) ([]byte, string, error) {
	var audio *gencontent.InlineData
	var texts []string
	for _, p := range res.FinalParts() {
		if d := p.InlineData; d != nil && d.Data != "" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(d.MimeType)), "audio/") {
			audio = d
		} else if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	if audio == nil {
		msg := "the response contained no audio"
		if r := res.FinishReason(); r != "" {
			msg += " (finishReason " + r + ")"
		}
		if res.PromptFeedback.BlockReason != "" {
			msg += " (prompt blocked: " + res.PromptFeedback.BlockReason + ")"
		}
		if len(texts) > 0 {
			msg += "; the model said: " + strings.Join(texts, " ")
		}
		return nil, "", clierrors.WithExitCode(fmt.Errorf("%s", msg), clierrors.ExitRuntime)
	}
	data, err := base64.StdEncoding.DecodeString(audio.Data)
	if err != nil {
		return nil, "", fmt.Errorf("the audio is not valid base64: %w", err)
	}
	return data, audio.MimeType, nil
}
