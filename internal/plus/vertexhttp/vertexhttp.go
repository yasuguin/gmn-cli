// Package vertexhttp は Agent Platform（旧 Vertex AI）へ生の HTTP で送る 1 回分の接続を持つ。
//
// 本家の SDK に載らない呼び出し（models の一覧、generateContent）が共有する。
// 本家の rawhttp と同じく、認証・User-Agent・x-goog-user-project・--header・--timeout・
// --dry-run の診断ラッパーを揃える。
package vertexhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google-gemini/gemini-api-cli/internal/client"
	"github.com/google-gemini/gemini-api-cli/internal/config"
	"github.com/google-gemini/gemini-api-cli/internal/flagutil"
	"github.com/google-gemini/gemini-api-cli/internal/plus/backend"
	"github.com/google-gemini/gemini-api-cli/internal/sdk"
	"github.com/google-gemini/gemini-api-cli/internal/sdk/models/sdkerrors"
	"github.com/google-gemini/gemini-api-cli/internal/sdk/sdkinternal/hooks"
	"github.com/google-gemini/gemini-api-cli/internal/testclient"
	"github.com/spf13/cobra"
)

// Session は Agent Platform への 1 回分の接続。
type Session struct {
	baseURL     string
	accessToken string
	userProject string
	headers     http.Header
	timeout     time.Duration
	client      client.HTTPClient
	// DryRun のとき診断ラッパーは送らずに示すだけなので、応答は読めない。
	DryRun bool
}

// New は cmd のフラグ・環境から接続を組み立てる。
func New(cmd *cobra.Command) (*Session, error) {
	s := &Session{headers: http.Header{}, DryRun: client.IsDryRun(cmd)}

	// 接続先は --server-url のスキームとホストだけを使う。Apply は本家向けの
	// "{host}/v1beta1/projects/.../locations" を入れるので、パスは捨てて呼び出し側が付け直す。
	serverURL, _ := flagutil.GetStringFlag(cmd, "server-url")
	u, err := url.Parse(serverURL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return nil, flagutil.WithCLIValidation(fmt.Errorf("invalid server URL %q for Agent Platform", serverURL))
	}
	s.baseURL = u.Scheme + "://" + u.Host

	// Agent Platform は OAuth のアクセストークンだけを受け付ける。明示のトークンがあればそれを付け、
	// 無ければ下の backend.WrapHTTPClient が ADC のトークンをリクエストごとに付ける。
	s.accessToken, _ = config.ResolveRequestSecurityCredential(cmd, "access-token")
	s.userProject = resolveGlobalString(cmd, "user-project")

	if hdrs, _ := flagutil.GetStringArrayFlag(cmd, "header"); len(hdrs) > 0 {
		parsed, err := flagutil.ParseHeaderFlags(hdrs)
		if err != nil {
			return nil, err
		}
		s.headers = parsed
	}

	timeoutStr, changed := flagutil.GetStringFlag(cmd, "timeout")
	if !changed || timeoutStr == "" {
		timeoutStr = config.GetString("timeout")
	}
	if timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err != nil || d <= 0 {
			return nil, flagutil.WithCLIValidation(fmt.Errorf("invalid --timeout value %q: must be a positive Go duration", timeoutStr))
		}
		s.timeout = d
	}

	// 本家の rawhttp と同じく、リダイレクトは辿らない（認証ヘッダを別ホストへ渡さない）。
	var httpClient client.HTTPClient = &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	if tc := testclient.NewTestHTTPClient(); tc != nil {
		httpClient = tc
	}
	// SDK・rawhttp の経路と同じく、診断ラッパーの外側で ADC のトークンを付ける。
	s.client = backend.WrapHTTPClient(cmd, client.WrapClientForDiagnostics(cmd, httpClient))
	return s, nil
}

// GetJSON は GET して JSON を out に読む。--dry-run では読まない。
func (s *Session) GetJSON(ctx context.Context, p string, query url.Values, out any) error {
	full := s.baseURL + p
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	return s.do(ctx, http.MethodGet, full, nil, out)
}

// PostJSON は body を JSON で POST して応答の JSON を out に読む。--dry-run では読まない。
func (s *Session) PostJSON(ctx context.Context, p string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return s.do(ctx, http.MethodPost, s.baseURL+p, raw, out)
}

func (s *Session) do(ctx context.Context, method, full string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, full, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", hooks.UserAgent(sdk.New().SDKVersion))
	if s.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.accessToken)
	}
	if s.userProject != "" {
		req.Header.Set("x-goog-user-project", s.userProject)
	}
	for k, vals := range s.headers {
		req.Header.Del(k)
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	cancel := context.CancelFunc(func() {})
	if s.timeout > 0 {
		var timeoutCtx context.Context
		timeoutCtx, cancel = context.WithTimeout(req.Context(), s.timeout)
		req = req.WithContext(timeoutCtx)
	}
	defer cancel()

	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return sdkerrors.NewSDKDefaultError("API error occurred", res.StatusCode, string(raw), res)
	}
	if s.DryRun {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// resolveGlobalString は共通パラメータを フラグ > 環境変数・設定 の順で読む。
func resolveGlobalString(cmd *cobra.Command, name string) string {
	if flagutil.FlagChanged(cmd, name) {
		v, _ := flagutil.GetStringFlag(cmd, name)
		return v
	}
	return config.GetString(name)
}
