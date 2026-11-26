package plus

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// docsFlags は付くと help・usage・version の出力になるフラグ（長い名前）。
var docsFlags = map[string]bool{
	"help": true, "help-global": true, "usage": true, "version": true,
}

// docsShorthands は、コマンドが同じ短縮形を別の意味で定義していないときだけ案内になる短縮形。
var docsShorthands = map[string]string{"h": "help", "v": "version"}

// isDocsFlag は引数 a が、target で案内（help・usage・version）を出すフラグかを返す。
// 短縮形はコマンドの定義で解く。tts の -v は --voice で、--version ではない。
// help・version は cobra が実行直前に足すので、定義が無いときは既定の短縮形で判断する。
func isDocsFlag(target *cobra.Command, a string) bool {
	switch {
	case strings.HasPrefix(a, "--"):
		name, _, _ := strings.Cut(a[2:], "=")
		return docsFlags[name]
	case len(a) == 2 && a[0] == '-':
		if f := lookupShorthand(target, a[1:]); f != nil {
			return docsFlags[f.Name]
		}
		return docsShorthands[a[1:]] != ""
	}
	return false
}

func lookupShorthand(c *cobra.Command, sh string) *pflag.Flag {
	if f := c.Flags().ShorthandLookup(sh); f != nil {
		return f
	}
	return c.InheritedFlags().ShorthandLookup(sh)
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
	target, _, err := root.Find(args)
	if err != nil || target == nil {
		target = root
	}
	for _, a := range args {
		if a == "--" {
			break
		}
		if isDocsFlag(target, a) {
			return true
		}
	}
	if target == root && err != nil {
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
