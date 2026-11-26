// Package gcsinput は --input に渡す Cloud Storage の URI（gs://）を扱う。
//
// Agent Platform には Files API が無いので、大きなファイルは Cloud Storage に置いて
// gs:// の URI を file data として渡す。URI は読まずに送るので、ローカルの探索や
// inline の上限とは関係がない。
package gcsinput

import (
	"path/filepath"
	"strings"
)

// scheme は Cloud Storage の URI の接頭辞。
const scheme = "gs://"

// IsURI は arg が gs:// で始まる Cloud Storage の URI かを返す。
func IsURI(arg string) bool {
	return strings.HasPrefix(arg, scheme)
}

// UploadHint は inline の上限を超えるローカルファイルを gs:// に置いて渡す直し方。
// Agent Platform では Files API の案内が使えないので、これを代わりに示す。
func UploadHint(localPath string) string {
	name := filepath.Base(localPath)
	return "Upload it to Cloud Storage and pass its URI instead: gcloud storage cp " + localPath +
		" gs://BUCKET/" + name + ", then pass --input gs://BUCKET/" + name
}
