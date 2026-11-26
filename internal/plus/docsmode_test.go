package plus

import (
	"testing"

	"github.com/spf13/cobra"
)

// 本家の構造に合わせた最小の木。tts の -v は --voice で、--version ではない。
func docsTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "gemini-headless", Version: "0.0.0"}
	tts := &cobra.Command{Use: "tts", Run: func(*cobra.Command, []string) {}}
	tts.Flags().StringP("voice", "v", "", "")
	generate := &cobra.Command{Use: "generate", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(tts, generate)
	return root
}

func TestStdoutIsDocsはフラグをコマンドの定義で解釈する(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"ttsの-vは声でありversionではない", []string{"tts", "-v", "Kore", "hello"}, false},
		{"ttsの--voice", []string{"tts", "--voice", "Kore", "hello"}, false},
		{"ttsの--help", []string{"tts", "--help"}, true},
		{"ttsの-h", []string{"tts", "-h"}, true},
		{"generateの--version", []string{"generate", "--version"}, true},
		{"generateの-v は未定義なので従来どおり案内とみなす", []string{"generate", "-v"}, true},
		{"rootの--version", []string{"--version"}, true},
		{"generateの通常実行", []string{"generate", "hi"}, false},
		{"-- 以降は見ない", []string{"generate", "--", "--help"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StdoutIsDocs(docsTestRoot(), c.args); got != c.want {
				t.Errorf("StdoutIsDocs(%v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}
