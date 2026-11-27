package capability_test

import (
	"testing"

	"github.com/google-gemini/gemini-api-cli/internal/cli"
	"github.com/google-gemini/gemini-api-cli/internal/gmn/capability"
	"github.com/spf13/cobra"
)

func walk(c *cobra.Command, visit func(*cobra.Command)) {
	for _, child := range c.Commands() {
		visit(child)
		walk(child, visit)
	}
}

func TestLookup子コマンドは近い親の扱いを継ぎ表に無ければ未確認(t *testing.T) {
	cases := []struct {
		path string
		kind capability.Kind
		key  string
	}{
		{"agent run", capability.Interactions, "agent"},
		{"files upload", capability.GCS, "files upload"},
		{"files list", capability.Unsupported, "files"},
		{"environments files upload", capability.Unsupported, "environments"},
		{"video", capability.Unverified, "video"},
		{"本家に新しく増えたコマンド", capability.Unverified, ""},
	}
	for _, c := range cases {
		e, key := capability.Lookup(c.path)
		if e.Kind != c.kind || key != c.key {
			t.Errorf("Lookup(%q) = %v, %q; want %v, %q", c.path, e.Kind, key, c.kind, c.key)
		}
	}
}

// 本家の追従でコマンドが増えたとき、表に載せ忘れて「未確認」のまま素通りするのを防ぐ。
func TestすべてのコマンドがAgentPlatformでの扱いを表に持つ(t *testing.T) {
	root, err := cli.NewRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	walk(root, func(c *cobra.Command) {
		path := capability.PathOf(c)
		if _, key := capability.Lookup(path); key == "" {
			t.Errorf("%q が能力表（internal/gmn/capability）に無い", path)
		}
	})
}
