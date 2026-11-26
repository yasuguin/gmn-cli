package contract_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google-gemini/gemini-api-cli/internal/cli"
	"github.com/google-gemini/gemini-api-cli/internal/plus"
	"github.com/spf13/cobra"
)

// 利用者向けの文書（skill と README）の ```bash ブロックに出る gemini-api-plus の呼び出しが、
// 実在するコマンドパスと実在するフラグを指すことを確かめる。実行はしない（ネットワークに出ない）。
// 文書だけ先に直して CLI を直し忘れる、またはその逆を防ぐ。
//
// README.ja.md は英語版から生成する途中の期間には無いことがあるので、あるときだけ検査する。

// plusDocsFiles は検査対象の文書（repo ルートからの相対パス）。
var plusDocsFiles = []string{
	"skills/gemini-api-plus/SKILL.md",
	".github/README.md",
	".github/README.ja.md",
}

const plusCLIName = "gemini-api-plus"

type docExample struct {
	file string
	line int
	raw  string
	toks []shellToken
}

type shellToken struct {
	text   string
	quoted bool // クォートを含む、または引用の中にある（フラグとして扱わない）
}

func TestPlusDocsバッシュブロックのコマンドは実在するコマンドパスとフラグを指す(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	examples := []docExample{}
	for _, rel := range plusDocsFiles {
		path := filepath.Join(root, rel)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		examples = append(examples, collectPlusDocExamples(t, path, rel)...)
	}
	// skill と英語版 README は必ず例を持つ。空なら抽出が壊れている。
	for _, rel := range []string{"skills/gemini-api-plus/SKILL.md", ".github/README.md"} {
		n := 0
		for _, ex := range examples {
			if ex.file == rel {
				n++
			}
		}
		if n == 0 {
			t.Fatalf("%s に検査対象の gemini-api-plus の例が無い", rel)
		}
	}

	// fork の --backend などは plus が有効のときだけ登録されるので、先に有効にする。
	plus.Enable()
	rootCmd, err := cli.NewRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	for _, ex := range examples {
		ex := ex
		t.Run(ex.file+":"+strconv.Itoa(ex.line), func(t *testing.T) {
			if err := checkPlusDocExample(rootCmd, ex); err != nil {
				t.Errorf("%s:%d: %s\n  %v", ex.file, ex.line, ex.raw, err)
			}
		})
	}
}

func TestPlusDocs英語版のREADMEは日本語版へリンクし日本語版があれば英語版へ戻る(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	en := readFileOrFatal(t, filepath.Join(root, ".github", "README.md"))
	if !strings.Contains(en, "](README.ja.md)") {
		t.Errorf(".github/README.md に README.ja.md への言語切り替えリンクが無い")
	}
	jaPath := filepath.Join(root, ".github", "README.ja.md")
	if _, err := os.Stat(jaPath); os.IsNotExist(err) {
		t.Skip(".github/README.ja.md はまだ無い（英語版から後で生成する）")
	}
	ja := readFileOrFatal(t, jaPath)
	if !strings.Contains(ja, "](README.md)") {
		t.Errorf(".github/README.ja.md に README.md への言語切り替えリンクが無い")
	}
}

func TestPlusDocsSkillのnameはディレクトリ名と一致する(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	body := readFileOrFatal(t, filepath.Join(root, "skills", "gemini-api-plus", "SKILL.md"))
	m := regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`).FindStringSubmatch(body)
	if m == nil || m[1] != "gemini-api-plus" {
		t.Errorf("SKILL.md の frontmatter の name が gemini-api-plus ではない: %v", m)
	}
}

// collectPlusDocExamples は文書の ```bash ブロックから、コマンド名で始まる行を集める。
func collectPlusDocExamples(t *testing.T, path, rel string) []docExample {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var out []docExample
	sc := bufio.NewScanner(f)
	in := false
	n := 0
	for sc.Scan() {
		n++
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if !in {
			if trimmed == "```bash" {
				in = true
			}
			continue
		}
		if trimmed == "```" {
			in = false
			continue
		}
		text := strings.TrimPrefix(trimmed, "$ ")
		if text != plusCLIName && !strings.HasPrefix(text, plusCLIName+" ") {
			continue
		}
		toks := shellTokens(text)
		out = append(out, docExample{file: rel, line: n, raw: trimmed, toks: toks})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// checkPlusDocExample は 1 行の例がコマンドパスとフラグを実在のものとして指すかを確かめる。
func checkPlusDocExample(root *cobra.Command, ex docExample) error {
	toks := ex.toks
	if len(toks) == 0 || toks[0].text != plusCLIName {
		return nil
	}
	cur := root
	i := 1
	// コマンドパスは、クォートなしの語が子コマンドの名前と一致する間だけたどる。
	for ; i < len(toks); i++ {
		tk := toks[i]
		if tk.quoted || strings.HasPrefix(tk.text, "-") {
			break
		}
		child := findChildCommand(cur, tk.text)
		if child == nil {
			break
		}
		cur = child
	}
	// 子コマンドを持つコマンドに、知らない語が続くのは誤り（引数を取らないコマンドの位置引数は別）。
	if i < len(toks) && !toks[i].quoted && !strings.HasPrefix(toks[i].text, "-") && cur.HasAvailableSubCommands() {
		return &docError{msg: "コマンド " + cur.CommandPath() + " に " + toks[i].text + " という子コマンドは無い"}
	}

	for ; i < len(toks); i++ {
		tk := toks[i]
		if tk.quoted || !strings.HasPrefix(tk.text, "-") || tk.text == "-" || tk.text == "--" {
			continue
		}
		name := strings.TrimLeft(tk.text, "-")
		if eq := strings.Index(name, "="); eq >= 0 {
			name = name[:eq]
		}
		if !hasFlag(cur, name, strings.HasPrefix(tk.text, "--")) {
			return &docError{msg: cur.CommandPath() + " に " + tk.text + " というフラグは無い"}
		}
	}
	return nil
}

// hasFlag は cur が name（長い形なら --name、短い形なら -n）のフラグを持つかを返す。
// help・version は cobra が後から足すので、名前だけで認める。
func hasFlag(cur *cobra.Command, name string, long bool) bool {
	if name == "help" || name == "h" {
		return true
	}
	if long {
		return cur.LocalFlags().Lookup(name) != nil || cur.InheritedFlags().Lookup(name) != nil
	}
	if len(name) != 1 {
		return false
	}
	return cur.LocalFlags().ShorthandLookup(name) != nil || cur.InheritedFlags().ShorthandLookup(name) != nil
}

func findChildCommand(cur *cobra.Command, name string) *cobra.Command {
	for _, c := range cur.Commands() {
		if c.Name() == name {
			return c
		}
		for _, a := range c.Aliases {
			if a == name {
				return c
			}
		}
	}
	return nil
}

type docError struct{ msg string }

func (e *docError) Error() string { return e.msg }

// shellTokens は 1 行をシェルの語に分ける。引用の中の空白は区切らず、引用を含む語は quoted にする。
// パイプ・論理演算子・リダイレクト・コメントに来たら、そこから後は例の一部ではない。
func shellTokens(line string) []shellToken {
	var out []shellToken
	var cur strings.Builder
	quoted := false
	var q rune
	flush := func() {
		if cur.Len() > 0 || quoted {
			out = append(out, shellToken{text: cur.String(), quoted: quoted})
		}
		cur.Reset()
		quoted = false
	}
	for _, r := range line {
		switch {
		case q != 0:
			if r == q {
				q = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			q = r
			quoted = true
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()

	for i, tk := range out {
		if tk.quoted {
			continue
		}
		if tk.text == "|" || tk.text == "&&" || tk.text == "||" || tk.text == ";" ||
			strings.HasPrefix(tk.text, "#") || strings.HasPrefix(tk.text, ">") || strings.Contains(tk.text, ">") {
			return out[:i]
		}
	}
	return out
}

func readFileOrFatal(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
