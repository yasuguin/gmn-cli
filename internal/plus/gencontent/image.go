package gencontent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google-gemini/gemini-api-cli/internal/clierrors"
	"github.com/google-gemini/gemini-api-cli/internal/output"
	"github.com/google-gemini/gemini-api-cli/internal/plus"
	"github.com/google-gemini/gemini-api-cli/internal/plus/backend"
	"github.com/google-gemini/gemini-api-cli/internal/plus/capability"
	"github.com/google-gemini/gemini-api-cli/internal/plus/modelmap"
	"github.com/spf13/cobra"
)

// TryImage は Agent Platform の image を generateContent で実行し、実行したら handled=true を返す。
// 本家の image は Interactions の本文（body）を組んで送るので、その本文を generateContent の形に写す。
// Gemini API のとき・image 以外のコマンドのとき・Interactions で通るモデルのときは何もしない。
func TryImage(cmd *cobra.Command, body any) (handled bool, err error) {
	if !plus.Enabled() || cmd.Name() != "image" {
		return false, nil
	}
	if s, rerr := backend.Resolve(cmd); rerr != nil || !s.IsVertex() {
		return false, nil
	}
	if e, _ := capability.Lookup(capability.PathOf(cmd)); e.Kind != capability.GenerateContent {
		return false, nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return true, err
	}
	var req map[string]any
	if err := json.Unmarshal(raw, &req); err != nil {
		return true, err
	}
	model, _ := req["model"].(string)
	model = strings.TrimPrefix(model, "models/")
	// 表に無い image のモデルも、Interactions では通らないので generateContent に送る。
	if st := modelmap.Lookup(model).Status; st != modelmap.GenerateContent && st != modelmap.Unverified {
		return false, nil
	}

	gc, err := imageRequest(req)
	if err != nil {
		return true, clierrors.WithExitCode(err, clierrors.ExitUsage)
	}

	stop := output.StartRequestProgress(cmd)
	res, err := Call(cmd.Context(), cmd, model, gc)
	stop()
	if err != nil {
		return true, output.Error(cmd, err)
	}
	if res == nil { // --dry-run
		return true, nil
	}

	if raw, _ := cmd.Flags().GetBool("raw-response"); raw {
		var v any
		if err := json.Unmarshal(res.Raw, &v); err != nil {
			return true, err
		}
		return true, output.Result(cmd, v)
	}
	img, err := finalImage(res)
	if err != nil {
		return true, output.Error(cmd, err)
	}
	// 本家の image の書き出し（--out・既定のファイル名・拡張子・envelope）に載せるため、
	// Interactions の応答の形に写して渡す。成果物は最終画像 1 枚だけ。
	out := map[string]any{
		"status": "completed",
		"steps": []any{map[string]any{
			"type": "model_output",
			"content": []any{map[string]any{
				"type": "image", "mime_type": img.MimeType, "data": img.Data,
			}},
		}},
	}
	if res.ResponseID != "" {
		out["id"] = res.ResponseID
	}
	return true, output.Result(cmd, out)
}

// finalImage は応答の最後の thought でない画像を返す。無ければ、応答のテキストと終了理由を含むエラーにする。
func finalImage(res *Response) (*InlineData, error) {
	var img *InlineData
	var texts []string
	for _, p := range res.FinalParts() {
		if d := p.InlineData; d != nil && d.Data != "" && strings.HasPrefix(d.MimeType, "image/") {
			img = d
		} else if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	if img != nil {
		return img, nil
	}
	msg := "the response contained no image"
	if r := res.FinishReason(); r != "" {
		msg += " (finishReason " + r + ")"
	}
	if res.PromptFeedback.BlockReason != "" {
		msg += " (prompt blocked: " + res.PromptFeedback.BlockReason + ")"
	}
	if len(texts) > 0 {
		msg += "; the model said: " + strings.Join(texts, " ")
	}
	return nil, fmt.Errorf("%s", msg)
}

// imageRequest は Interactions の image 用の本文を generateContent の本文にする。
// 写せない項目は黙って捨てず、Agent Platform では使えないと伝える。
func imageRequest(req map[string]any) (map[string]any, error) {
	for k := range req {
		switch k {
		case "model", "input", "response_format", "stream":
		case "previous_interaction_id":
			return nil, &usageError{
				msg: "--previous-interaction-id is not available on Agent Platform: generateContent has no conversation ID",
				hints: []string{
					"To edit an image, pass it with --input (a local file or a gs:// URI) together with the instruction",
					"To continue from an Interactions API image, switch to the Gemini API: pass --backend gemini",
				},
			}
		default:
			return nil, &usageError{
				msg:   fmt.Sprintf("%q in the request body is not supported for image on Agent Platform", k),
				hints: []string{"Use --backend gemini to send the full Interactions API request"},
			}
		}
	}

	parts, err := inputParts(req["input"])
	if err != nil {
		return nil, err
	}
	genCfg := map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}

	if rf, ok := req["response_format"].(map[string]any); ok {
		cfg := map[string]any{}
		if v, ok := rf["aspect_ratio"]; ok {
			cfg["aspectRatio"] = v
		}
		if v, ok := rf["image_size"]; ok {
			cfg["imageSize"] = v
		}
		if v, ok := rf["mime_type"]; ok {
			cfg["imageOutputOptions"] = map[string]any{"mimeType": v}
		}
		if len(cfg) > 0 {
			genCfg["imageConfig"] = cfg
		}
	}
	return map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": parts}},
		"generationConfig": genCfg,
	}, nil
}

// inputParts は Interactions の input（文字列・content の配列・user_input の step の配列）を parts にする。
func inputParts(input any) ([]any, error) {
	switch v := input.(type) {
	case string:
		return []any{map[string]any{"text": v}}, nil
	case []any:
		var parts []any
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, &usageError{msg: "unsupported input item for image on Agent Platform"}
			}
			if m["type"] == "user_input" {
				inner, err := inputParts(m["content"])
				if err != nil {
					return nil, err
				}
				parts = append(parts, inner...)
				continue
			}
			p, err := contentPart(m)
			if err != nil {
				return nil, err
			}
			parts = append(parts, p)
		}
		return parts, nil
	}
	return nil, &usageError{msg: "unsupported input for image on Agent Platform"}
}

func contentPart(m map[string]any) (map[string]any, error) {
	typ, _ := m["type"].(string)
	if typ == "text" {
		return map[string]any{"text": m["text"]}, nil
	}
	mime, _ := m["mime_type"].(string)
	if data, ok := m["data"].(string); ok && data != "" {
		return map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": data}}, nil
	}
	if uri, ok := m["uri"].(string); ok && uri != "" {
		return map[string]any{"fileData": map[string]any{"mimeType": mime, "fileUri": uri}}, nil
	}
	return nil, &usageError{msg: fmt.Sprintf("unsupported %q input for image on Agent Platform", typ)}
}

// usageError は呼び方の誤りを示す CLI_VALIDATION の構造化エラー。
type usageError struct {
	msg   string
	hints []string
}

func (e *usageError) Error() string          { return e.msg }
func (e *usageError) CLIReason() string      { return "CLI_VALIDATION" }
func (e *usageError) CLIHints() []string     { return e.hints }
func (e *usageError) CLIOmitTypeHints() bool { return true }
