// Package gencontent は Agent Platform（旧 Vertex AI）の generateContent を呼ぶ共通部品。
//
// Interactions API が通さないモデル（image・tts）はこちらで呼ぶ。コマンドごとの変換（#11 の image は image.go、
// #12 の tts はそれに倣って別ファイルに足す）は、Call に generateContent の本文を渡し、
// 返った Response の FinalParts から欲しい part を取り出す。
// 認証・--header・--timeout・--dry-run は vertexhttp の Session が担う。
package gencontent

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/google-gemini/gemini-api-cli/internal/plus/backend"
	"github.com/google-gemini/gemini-api-cli/internal/plus/vertexhttp"
	"github.com/spf13/cobra"
)

// APIVersion は generateContent の版。Interactions の v1beta1 と違い、smoke で v1 が通った。
const APIVersion = "v1"

// Response は generateContent の応答のうち、写しに使う項目と、そのままの本文。
type Response struct {
	// Raw は応答の本文そのまま（--raw-response 用）。
	Raw        json.RawMessage `json:"-"`
	ResponseID string          `json:"responseId"`
	Candidates []Candidate     `json:"candidates"`
	// PromptFeedback はプロンプトが拒否されたとき（candidates が空）の理由。
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

// Candidate は応答の候補 1 件。
type Candidate struct {
	FinishReason string `json:"finishReason"`
	Content      struct {
		Parts []Part `json:"parts"`
	} `json:"content"`
}

// Part は応答の part。Thought が true のものは途中経過（考えのテキストや下書き画像）で、成果物ではない。
type Part struct {
	Text       string      `json:"text"`
	Thought    bool        `json:"thought"`
	InlineData *InlineData `json:"inlineData"`
}

// InlineData は base64 の中身と MIME type。
type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// FinalParts は先頭の候補から thought でない part を順に返す。
func (r *Response) FinalParts() []Part {
	if len(r.Candidates) == 0 {
		return nil
	}
	var out []Part
	for _, p := range r.Candidates[0].Content.Parts {
		if !p.Thought {
			out = append(out, p)
		}
	}
	return out
}

// FinishReason は先頭の候補の終了理由。候補が無ければ空。
func (r *Response) FinishReason() string {
	if len(r.Candidates) == 0 {
		return ""
	}
	return r.Candidates[0].FinishReason
}

// Path は generateContent の URL パス。接続先のホストは Session が持つ。
func Path(project, location, model string) string {
	return "/" + APIVersion + "/projects/" + url.PathEscape(project) + "/locations/" + url.PathEscape(location) +
		"/publishers/google/models/" + url.PathEscape(model) + ":generateContent"
}

// Call は model に body を generateContent で送る。--dry-run のときは送らずに示すだけで、応答は空（nil, nil）。
func Call(ctx context.Context, cmd *cobra.Command, model string, body map[string]any) (*Response, error) {
	b, err := backend.Resolve(cmd)
	if err != nil {
		return nil, err
	}
	s, err := vertexhttp.New(cmd)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := s.PostJSON(ctx, Path(b.Project, b.Location, model), body, &raw); err != nil {
		return nil, err
	}
	if s.DryRun {
		return nil, nil
	}
	res := &Response{Raw: raw}
	if err := json.Unmarshal(raw, res); err != nil {
		return nil, err
	}
	return res, nil
}
