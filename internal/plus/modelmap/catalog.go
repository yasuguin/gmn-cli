package modelmap

import (
	"fmt"
	"strings"

	"github.com/google-gemini/gemini-api-cli/internal/output"
	"github.com/spf13/cobra"
)

// statusText は Agent Platform でのモデルの状態を人が読む 1 行にする。
func statusText(id string) string {
	e := Lookup(id)
	switch e.Status {
	case Verified:
		return "Agent Platform: works as is"
	case Renamed:
		return "Agent Platform: use " + e.Name
	case Unsupported:
		if len(e.Alternatives) == 0 {
			return "Agent Platform: not available"
		}
		return "Agent Platform: not available (use " + strings.Join(e.Alternatives, ", ") + ")"
	case GenerateContent:
		return "Agent Platform: generateContent only (not available via Interactions API)"
	}
	return "Agent Platform: unverified (sent as is)"
}

// PrintCatalog はオフラインのカタログ（models）に Agent Platform での状態を足して出し、出したら true を返す（出力に失敗したら error も返す）。
// Gemini API のときは false を返し、本家の出力に任せる。
func PrintCatalog(cmd *cobra.Command, values []map[string]interface{}) (bool, error) {
	if !onAgentPlatform(cmd) {
		return false, nil
	}
	if output.IsMachineMode(cmd) {
		out := make([]map[string]interface{}, 0, len(values))
		for _, v := range values {
			id, _ := v["value"].(string)
			e := Lookup(id)
			info := map[string]interface{}{"status": string(e.Status)}
			if e.Name != "" {
				info["name"] = e.Name
			}
			if len(e.Alternatives) > 0 {
				info["alternatives"] = e.Alternatives
			}
			c := make(map[string]interface{}, len(v)+1)
			for k, x := range v {
				c[k] = x
			}
			c["agent_platform"] = info
			out = append(out, c)
		}
		return true, output.LocalResult(cmd, out)
	}
	w := cmd.OutOrStdout()
	last := ""
	for i, v := range values {
		group, _ := v["group"].(string)
		if group != last {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "%s:\n", group)
			last = group
		}
		id, _ := v["value"].(string)
		desc, _ := v["description"].(string)
		label := id
		if d, _ := v["default"].(bool); d {
			label += " (default)"
		} else if df, ok := v["default_for"].([]string); ok && len(df) > 0 {
			label += " (default: " + strings.Join(df, ", ") + ")"
		}
		fmt.Fprintf(w, "  %-42s %s\n", label, desc)
		fmt.Fprintf(w, "  %-42s [%s]\n", "", statusText(id))
	}
	return true, nil
}
