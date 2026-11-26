// gemini-headless は gemini-api の fork 用エントリポイント。
// 本家の cmd/gemini-api は scripts/validate.sh が go install するので残し、挙動を変えない。
package main

import (
	"os"

	"github.com/google-gemini/gemini-api-cli/internal/plus/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr))
}
