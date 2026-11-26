package plus

import (
	"bytes"
	"io"
	"os"
	"sync"
)

// 置き換え対象の判定は前後 1 文字で行う。
//   - 前が名前に使える文字・'.'・'/' なら対象外（URL・パス・別名の一部を壊さない）。
//     ただし設定ファイルの場所 ".config/gemini-api/" は対象にする。
//   - 後ろが名前に使える文字なら対象外（gemini-api-cli、gemini-api-plus など）。
//   - 後ろが '/' なら設定ファイルの場所のときだけ対象にする。
const configDirPrefix = ".config/"

func isNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

// ReplaceCommandName は text 中のコマンド名 gemini-api を gemini-api-plus に置き換える。
func ReplaceCommandName(text string) string {
	return string(replaceCommandName(nil, []byte(text)))
}

// replaceCommandName は hist（直前までの原文）を前後関係の参照にして、
// chunk の置き換え結果を返す。
func replaceCommandName(hist, chunk []byte) []byte {
	full := append(append([]byte(nil), hist...), chunk...)
	var out bytes.Buffer
	i := len(hist)
	for i < len(full) {
		j := bytes.Index(full[i:], []byte(upstreamName))
		if j < 0 {
			out.Write(full[i:])
			break
		}
		start := i + j
		end := start + len(upstreamName)
		out.Write(full[i:start])
		inConfigDir := start >= len(configDirPrefix) && string(full[start-len(configDirPrefix):start]) == configDirPrefix
		okPrev := start == 0 || inConfigDir || !(isNameByte(full[start-1]) || full[start-1] == '.' || full[start-1] == '/')
		okNext := end == len(full) || !(isNameByte(full[end]) || full[end] == '/' && !inConfigDir)
		if okPrev && okNext {
			out.WriteString(Name)
		} else {
			out.WriteString(upstreamName)
		}
		i = end
	}
	return out.Bytes()
}

// holdLen は、次の書き込みで名前の続きが来るかもしれない末尾の長さを返す。
// gemini-api の先頭部分（全体を含む）で終わっているときだけ保留し、
// それ以外は即座に流してストリーミング出力を遅らせない。
func holdLen(b []byte) int {
	for k := min(len(upstreamName), len(b)); k > 0; k-- {
		if bytes.HasPrefix([]byte(upstreamName), b[len(b)-k:]) {
			return k
		}
	}
	return 0
}

// historyLen は前後関係の参照に残す直前の原文の長さ（".config/" の判定に足りる分）。
const historyLen = len(configDirPrefix) + 1

// Writer は書き込みのコマンド名を置き換えて dst へ流す。
// 末尾が名前の途中かもしれないときだけ保留するので、Flush を終了時に呼ぶこと。
type Writer struct {
	mu      sync.Mutex
	dst     io.Writer
	hist    []byte
	pending []byte
}

// NewWriter は dst への書き込みを置き換える Writer を返す。
func NewWriter(dst io.Writer) *Writer {
	return &Writer{dst: dst}
}

// File は色の端末判定（*os.File だけを端末候補にする呼び出し側）のために、
// dst がファイルならそれを返す。
func (w *Writer) File() *os.File {
	f, _ := w.dst.(*os.File)
	return f
}

// Fd は端末判定（Fd() を持つ writer だけを端末と見なす呼び出し側）のために dst の fd を返す。
func (w *Writer) Fd() uintptr {
	if f, ok := w.dst.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	return ^uintptr(0)
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	n := len(w.pending) - holdLen(w.pending)
	if err := w.emitLocked(w.pending[:n]); err != nil {
		return 0, err
	}
	w.pending = append([]byte(nil), w.pending[n:]...)
	return len(p), nil
}

// Flush は保留中の末尾を流す。
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked()
}

func (w *Writer) flushLocked() error {
	err := w.emitLocked(w.pending)
	w.pending = nil
	return err
}

func (w *Writer) emitLocked(chunk []byte) error {
	if len(chunk) == 0 {
		return nil
	}
	out := replaceCommandName(w.hist, chunk)
	w.hist = append(w.hist, chunk...)
	if len(w.hist) > historyLen {
		w.hist = append([]byte(nil), w.hist[len(w.hist)-historyLen:]...)
	}
	_, err := w.dst.Write(out)
	return err
}
