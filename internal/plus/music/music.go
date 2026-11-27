// Package music は、Agent Platform（旧 Vertex AI）の music を Interactions API が受け付ける形で送り、
// 応答を本家の書き出しが読める形に直す。
//
// Agent Platform の Interactions API と Lyria の組み合わせには、Gemini API と違う点が 3 つある（実測 2026-11-27、global）。
//   - response_format（type: audio）を付けると 400 で拒否する（付けないと「bit_rate must be positive」、
//     bit_rate を足すと「Audio bit_rate is not supported」）。付けなければ audio/mpeg を返す。
//   - 既定の lyria-3.5 を受け付けない。
//   - 応答は steps ではなく、古い形の outputs に音声を入れて返す。本家の書き出しは steps しか読まない。
package music

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/google-gemini/gemini-api-cli/internal/output"
	"github.com/google-gemini/gemini-api-cli/internal/plus"
	"github.com/google-gemini/gemini-api-cli/internal/plus/backend"
	"github.com/google-gemini/gemini-api-cli/internal/plus/modelmap"
	"github.com/google-gemini/gemini-api-cli/internal/plus/vertexhttp"
	"github.com/spf13/cobra"
)

// Try は Agent Platform の music のとき、本文を直して自分で送り、結果を書き出して handled を true で返す。
// それ以外のコマンド・Gemini API では何もせず false を返す。
// --model で明示した Agent Platform に無いモデルは、ここに来る前に modelmap.Guard が止める。
func Try(cmd *cobra.Command, body any) (handled bool, err error) {
	if !plus.Enabled() || cmd.Name() != "music" {
		return false, nil
	}
	s, rerr := backend.Resolve(cmd)
	if rerr != nil || !s.IsVertex() {
		return false, nil
	}
	req, err := toMap(body)
	if err != nil {
		return true, err
	}
	delete(req, "response_format")
	if !cmd.Flags().Changed("model") {
		if def, ok := req["model"].(string); ok {
			use, notice := modelmap.ResolveDefault(cmd, def)
			req["model"] = use
			if notice != "" && !output.IsMachineMode(cmd) {
				fmt.Fprintln(cmd.ErrOrStderr(), notice)
			}
		}
	}

	sess, err := vertexhttp.New(cmd)
	if err != nil {
		return true, err
	}
	stop := output.StartRequestProgress(cmd)
	var raw json.RawMessage
	err = sess.PostJSON(cmd.Context(), interactionsPath(s.Project, s.Location), req, &raw)
	stop()
	if err != nil {
		return true, output.Error(cmd, err)
	}
	if sess.DryRun {
		return true, nil
	}
	var res map[string]any
	if err := json.Unmarshal(raw, &res); err != nil {
		return true, err
	}
	if rawFlag, _ := cmd.Flags().GetBool("raw-response"); rawFlag {
		return true, output.Result(cmd, res)
	}
	return true, output.Result(cmd, withSteps(res))
}

// interactionsPath は Interactions API の作成の URL パス。接続先のホストは Session が持つ。
func interactionsPath(project, location string) string {
	return "/" + backend.APIVersion + "/projects/" + url.PathEscape(project) + "/locations/" + url.PathEscape(location) + "/interactions"
}

// withSteps は outputs だけを持つ応答に、同じ中身の steps（model_output 1 件）を足す。steps があれば触らない。
func withSteps(res map[string]any) map[string]any {
	if _, ok := res["steps"]; ok {
		return res
	}
	outs, ok := res["outputs"].([]any)
	if !ok {
		return res
	}
	res["steps"] = []any{map[string]any{"type": "model_output", "content": outs}}
	return res
}

func toMap(body any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}
