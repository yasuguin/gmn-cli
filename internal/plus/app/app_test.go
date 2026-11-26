package app_test

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/google-gemini/gemini-api-cli/internal/config"
	"github.com/google-gemini/gemini-api-cli/internal/plus/app"
)

type recordingKeyring struct {
	mu       sync.Mutex
	services map[string]bool
}

func (r *recordingKeyring) record(service string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services[service] = true
}

func (r *recordingKeyring) Get(service, _ string) (string, error) {
	r.record(service)
	return "", errors.New("secret not found in keyring")
}
func (r *recordingKeyring) Set(service, _, _ string) error { r.record(service); return nil }
func (r *recordingKeyring) Delete(service, _ string) error { r.record(service); return nil }

// キーチェーンのサービス名が公式の gemini-api と別になる。
func Testキーチェーンのサービス名は公式と別(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	config.Reset()
	t.Cleanup(config.Reset)
	rec := &recordingKeyring{services: map[string]bool{}}
	config.SetKeyringBackend(rec)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"--no-interactive", "version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version failed: %d\n%s", code, stderr.String())
	}
	if err := config.SetKeyringValue("api-key", "dummy"); err != nil {
		t.Fatal(err)
	}
	if !rec.services["gmn"] {
		t.Errorf("サービス名 gmn で保存されていない: %v", rec.services)
	}
	if rec.services["gemini-api"] {
		t.Errorf("公式のサービス名 gemini-api を使っている: %v", rec.services)
	}
}
