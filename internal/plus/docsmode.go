package plus

import "github.com/spf13/cobra"

// docsFlags は付くと help・usage・version の出力になるフラグ。
var docsFlags = map[string]bool{
	"--help": true, "-h": true, "--help-global": true,
	"--usage": true, "--version": true, "-v": true,
}

// docsCommands は標準出力が案内・設定の文面だけになるトップレベルのコマンド。
var docsCommands = map[string]bool{
	"help": true, "version": true, "completion": true,
	"whoami": true, "configure": true, "auth": true, "explore": true,
}

// StdoutIsDocs は、この呼び出しの標準出力が本家の案内文面かを返す。
// API を呼ぶコマンドの標準出力にはモデルの返答が載るので、そこでは
// コマンド名を置き換えない（返答の中の gemini-api を書き換えないため）。
func StdoutIsDocs(root *cobra.Command, args []string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		if docsFlags[a] {
			return true
		}
	}
	target, _, err := root.Find(args)
	if err != nil || target == nil {
		return false
	}
	if target.HasSubCommands() {
		return true
	}
	top := target
	for top.HasParent() && top.Parent().HasParent() {
		top = top.Parent()
	}
	return docsCommands[top.Name()]
}
