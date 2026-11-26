package backend

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/google-gemini/gemini-api-cli/internal/clierrors"
	"github.com/google-gemini/gemini-api-cli/internal/config"
	"github.com/google-gemini/gemini-api-cli/internal/flagutil"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// cloudPlatformScope は Agent Platform の API が要求する OAuth スコープ。
const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// authADC は whoami の認証の出どころ。明示トークンのときは flag・env・config・keyring。
const authADC = "adc"

// adcPending は「このプロセスの API リクエストの認証を ADC に任せる」かどうか。
// Apply が決め、WrapHTTPClient が読む。
var adcPending atomic.Bool

// projectMissing は Agent Platform なのに project が未設定、という状態。リクエストを送る時点で止める。
var projectMissing atomic.Bool

// missingProjectClient は何も送らず、project 未設定のエラーを返す。
type missingProjectClient struct{}

func (missingProjectClient) Do(*http.Request) (*http.Response, error) {
	return nil, flagutil.WithCLIValidation(missingProjectError{})
}

// Doer は HTTP リクエストを送れるもの。本家の client.HTTPClient と同じ形で、
// backend が client パッケージを import しなくて済むように別に定義している。
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// WrapHTTPClient は ADC を使うときだけ、リクエストごとに最新のアクセストークンを
// Authorization に付ける Doer で c を包む。ADC を使わないときは c をそのまま返す。
//
// --access-token にトークンを入れておく方式だと、1 回の実行の途中（動画生成のポーリング・
// 長い agent 実行）で期限が切れても更新できない。ここで毎回 TokenSource から取れば、
// oauth2.ReuseTokenSource が期限前に自動で取り直す。
// 本家の 3 つの HTTP 経路（SDK・rawhttp・環境ランタイム）のうち、SDK と rawhttp がこれを通る。
func WrapHTTPClient(cmd *cobra.Command, c Doer) Doer {
	if projectMissing.Load() {
		return missingProjectClient{}
	}
	if !adcPending.Load() {
		return c
	}
	dryRun, _ := flagutil.GetBoolFlag(cmd, "dry-run")
	return &adcClient{next: c, dryRun: dryRun, load: loadADC}
}

// loadADC は ADC を探して TokenSource を返す。
func loadADC(ctx context.Context) (oauth2.TokenSource, error) {
	creds, err := google.FindDefaultCredentials(ctx, cloudPlatformScope)
	if err != nil {
		return nil, err
	}
	return oauth2.ReuseTokenSource(nil, creds.TokenSource), nil
}

type adcClient struct {
	next   Doer
	dryRun bool
	load   func(context.Context) (oauth2.TokenSource, error)

	mu  sync.Mutex
	src oauth2.TokenSource
}

// tokenSource は ADC を最初に使うときに 1 回だけ読み、成功したものだけ覚える。
// 失敗を覚えると一時的な失敗のあとの全リクエストが失敗し続けるので、次のリクエストで再試行する。
// oauth2 は読み出しに渡した context を後のトークン更新にも使うため、リクエストの context ではなく
// プロセスの寿命の Background を渡す（リクエストが終わった context で約 1 時間後の更新が失敗するのを避ける）。
func (c *adcClient) tokenSource() (oauth2.TokenSource, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.src != nil {
		return c.src, nil
	}
	src, err := c.load(context.Background())
	if err != nil {
		return nil, err
	}
	c.src = src
	return src, nil
}

// dryRunAuthorization は --dry-run のプレビューに出す Authorization の値。
// ADC は読まないので実際のトークンは無い。
const dryRunAuthorization = "Bearer <ADC access token>"

func (c *adcClient) Do(req *http.Request) (*http.Response, error) {
	// 明示の Authorization（--access-token・--header）が付いているものは触らない。
	if req.Header.Get("Authorization") != "" {
		return c.next.Do(req)
	}
	if c.dryRun {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", dryRunAuthorization)
		return c.next.Do(req)
	}
	src, err := c.tokenSource()
	if err != nil {
		return nil, adcNotFoundError{err}
	}
	tok, err := src.Token()
	if err != nil {
		return nil, adcTokenError{err}
	}
	config.RegisterSecret(tok.AccessToken)
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	return c.next.Do(req)
}

// adcNotFoundError は ADC が見つからないときのエラー。本家の認証エラーの流儀で終了コード 3。
type adcNotFoundError struct{ cause error }

func (e adcNotFoundError) Error() string {
	return "no Application Default Credentials found for --backend vertex: run `gcloud auth application-default login`, " +
		"or set GOOGLE_APPLICATION_CREDENTIALS to a credentials file, or pass --access-token (" + e.cause.Error() + ")"
}
func (e adcNotFoundError) Unwrap() error { return e.cause }
func (adcNotFoundError) ExitCode() int   { return clierrors.ExitAuth }

// adcTokenError は ADC は見つかったがトークンを取れなかったとき（失効した refresh token など）のエラー。
type adcTokenError struct{ cause error }

func (e adcTokenError) Error() string {
	return "could not get an access token from Application Default Credentials: " + e.cause.Error() +
		" (run `gcloud auth application-default login` again, or check GOOGLE_APPLICATION_CREDENTIALS)"
}
func (e adcTokenError) Unwrap() error { return e.cause }
func (adcTokenError) ExitCode() int   { return clierrors.ExitAuth }
