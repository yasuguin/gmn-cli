// Package plus は fork（gemini-api-plus）の独自挙動をまとめる。
//
// 本家のファイルには呼び出す行だけを足し、その行に "// plus:" のコメントを付ける。
// 本家の cmd/gemini-api では Enable が呼ばれないので、挙動は本家のまま変わらない。
package plus

import (
	"sync/atomic"
)

// Name は fork のコマンド名。公式の gemini-api と並べて入れても混ざらないようにする。
const Name = "gemini-api-plus"

// Version は fork の版。リリース時は ldflags の -X で上書きする。
var Version = "0.1.0-dev"

// upstreamName は本家のコマンド名で、画面に出る前に Name へ置き換える対象。
const upstreamName = "gemini-api"

// baseProject は --version の「土台」表示に使う本家の名前。
// upstreamName を含めない（含めると出力の置き換えで Name に化ける）。
const baseProject = "gemini-api-cli"

var enabled atomic.Bool

// Enable は fork の挙動を有効にする。cmd/gemini-api-plus だけが呼ぶ。
func Enable() { enabled.Store(true) }

// Enabled は fork の挙動が有効かを返す。
func Enabled() bool { return enabled.Load() }

// UserAgent は本家形式の User-Agent の先頭に fork 名と版を足す。
// 本家の名前は残し、サーバー側で土台の版も追えるようにする。
func UserAgent(upstream string) string {
	if !Enabled() {
		return upstream
	}
	return Name + "/" + Version + " " + upstream
}

// VersionHeader は --version の 1 行目を返す。baseVersion は本家の Version 定数。
func VersionHeader(upstream, baseVersion string) string {
	if !Enabled() {
		return upstream
	}
	return Name + " " + Version + " (base " + baseProject + " " + baseVersion + ")"
}

// AnnotateVersionInfo は --version の機械可読出力に fork の版と土台の版を入れる。
func AnnotateVersionInfo(info map[string]any, baseVersion string) {
	if !Enabled() {
		return
	}
	info["name"] = Name
	info["version"] = Version
	info["base_version"] = baseVersion
}
