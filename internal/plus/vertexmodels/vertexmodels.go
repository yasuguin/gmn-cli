// Package vertexmodels は Agent Platform（旧 Vertex AI）での models list・get を担う。
//
// Agent Platform の models には Gemini API の models.list・models.get に当たる公開エンドポイントが
// 無い。projects.locations.models はプロジェクトが登録したモデルの一覧なので使わず、Google 提供の
// 基盤モデルの一覧 publishers/google/models を叩く。応答は Gemini API の models の形に写し、
// 本家と同じ出力処理に渡す。
package vertexmodels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google-gemini/gemini-api-cli/internal/client"
	"github.com/google-gemini/gemini-api-cli/internal/config"
	"github.com/google-gemini/gemini-api-cli/internal/flagutil"
	"github.com/google-gemini/gemini-api-cli/internal/output"
	"github.com/google-gemini/gemini-api-cli/internal/plus/backend"
	"github.com/google-gemini/gemini-api-cli/internal/sdk"
	"github.com/google-gemini/gemini-api-cli/internal/sdk/models/genai"
	"github.com/google-gemini/gemini-api-cli/internal/sdk/models/operations"
	"github.com/google-gemini/gemini-api-cli/internal/sdk/models/sdkerrors"
	"github.com/google-gemini/gemini-api-cli/internal/sdk/sdkinternal/hooks"
	"github.com/google-gemini/gemini-api-cli/internal/testclient"
	"github.com/spf13/cobra"
)

// maxPageSize は Agent Platform の models list が受け付けた page size の上限。
// 上限超の 400（"maximum size is 100"）を通信前に防ぐ。
const maxPageSize = 100

// publisherModelsPath は Google 提供の基盤モデルの一覧（publishers.models.list）。
// Gemini API の /models と同じ役割を持つ。
const publisherModelsPath = "/v1beta1/publishers/google/models"

// publisherModel は publishers.models の応答のうち、写しに使う項目だけ。
// displayName・inputTokenLimit・supportedGenerationMethods などは応答に無いので写さない。
type publisherModel struct {
	Name string `json:"name"`
}

type publisherModelList struct {
	PublisherModels []publisherModel `json:"publisherModels"`
	NextPageToken   string           `json:"nextPageToken"`
}

// Enabled は Agent Platform が選ばれているかを返す。本家の models コマンドはこれで振り分ける。
func Enabled() bool {
	s := backend.Current()
	return s != nil && s.IsVertex()
}

// RunList は models list を Agent Platform で実行する。--page-size の検査・ページ送り・出力は本家と同じ処理に載せる。
func RunList(cmd *cobra.Command) error {
	pageSize, err := pageSizeFlag(cmd)
	if err != nil {
		return err
	}
	token, _ := flagutil.GetStringFlag(cmd, "page-token")
	s, err := newSession(cmd)
	if err != nil {
		return err
	}
	res, err := s.listResponse(cmd.Context(), pageSize, token)
	if err != nil {
		return output.Error(cmd, err)
	}
	allPages, _ := flagutil.GetBoolFlag(cmd, "all")
	if allPages && !client.IsDryRun(cmd) {
		maxPages, _ := flagutil.GetIntFlag(cmd, "max-pages")
		return output.PaginatedResult(cmd, res, "ListModelsResponse", "", maxPages, output.PaginationProbe{
			Type:       "cursor",
			CursorKind: "string",
			NextCursor: "$.nextPageToken",
		})
	}
	if err := output.Result(cmd, res); err != nil {
		return err
	}
	next := res.ListModelsResponse.NextPageToken
	if next != nil && !client.IsDryRun(cmd) && !output.IsMachineMode(cmd) {
		fmt.Fprintln(cmd.ErrOrStderr(), "Hint: more pages available. Use --all to fetch all results, or --page-token for manual pagination.")
	}
	return nil
}

// RunGet は models get を Agent Platform で実行する。"models/" の有無はどちらでも同じモデルを指す。
func RunGet(cmd *cobra.Command) error {
	model, _ := flagutil.GetStringFlag(cmd, "model")
	id := strings.TrimPrefix(strings.TrimSpace(model), "models/")
	s, err := newSession(cmd)
	if err != nil {
		return err
	}
	var body publisherModel
	if err := s.getJSON(cmd.Context(), publisherModelsPath+"/"+url.PathEscape(id), nil, &body); err != nil {
		return output.Error(cmd, err)
	}
	return output.Result(cmd, &operations.ModelsGetResponse{
		Model: &genai.Model{Name: strPtr("models/" + id)},
	})
}

// listResponse は 1 ページ分を Gemini API の形の envelope にする。
// Next は SDK と同じく次のページを返す closure で、--all のページ送り（output.PaginatedResult）が呼ぶ。
func (s *session) listResponse(ctx context.Context, pageSize int64, token string) (*operations.ModelsListResponse, error) {
	q := url.Values{}
	q.Set("pageSize", strconv.FormatInt(pageSize, 10))
	if token != "" {
		q.Set("pageToken", token)
	}
	var body publisherModelList
	if err := s.getJSON(ctx, publisherModelsPath, q, &body); err != nil {
		return nil, err
	}
	models := make([]genai.Model, 0, len(body.PublisherModels))
	for _, m := range body.PublisherModels {
		id := path.Base(m.Name)
		if id == "" || id == "." || id == "/" {
			continue
		}
		models = append(models, genai.Model{Name: strPtr("models/" + id)})
	}
	env := &operations.ModelsListResponse{
		ListModelsResponse: &genai.ListModelsResponse{Models: models},
	}
	if body.NextPageToken != "" {
		next := body.NextPageToken
		env.ListModelsResponse.NextPageToken = strPtr(next)
		env.Next = func() (*operations.ModelsListResponse, error) {
			return s.listResponse(ctx, pageSize, next)
		}
	}
	return env, nil
}

// pageSizeFlag は --page-size を検査する。上限を超えると通信前に usage エラー（終了コード 2）にする。
// 既定は本家の 1000 ではなく上限の 100 を送る。
func pageSizeFlag(cmd *cobra.Command) (int64, error) {
	if !flagutil.FlagChanged(cmd, "page-size") {
		return maxPageSize, nil
	}
	v, _ := flagutil.GetInt64Flag(cmd, "page-size")
	if v < 1 || v > maxPageSize {
		return 0, flagutil.WithCLIValidation(fmt.Errorf("--page-size must be between 1 and %d on Agent Platform (got %d)", maxPageSize, v))
	}
	return v, nil
}

// session は Agent Platform への 1 回分の接続。本家の rawhttp と同じく、認証・User-Agent・
// x-goog-user-project・--header・--timeout・--dry-run の診断ラッパーを揃える。
type session struct {
	baseURL     string
	accessToken string
	userProject string
	headers     http.Header
	timeout     time.Duration
	client      client.HTTPClient
	dryRun      bool
}

func newSession(cmd *cobra.Command) (*session, error) {
	s := &session{headers: http.Header{}, dryRun: client.IsDryRun(cmd)}

	// 接続先は --server-url のスキームとホストだけを使う。Apply は本家向けの
	// "{host}/v1beta1/projects/.../locations" を入れるので、パスは捨てて基盤モデル用に付け直す。
	serverURL, _ := flagutil.GetStringFlag(cmd, "server-url")
	u, err := url.Parse(serverURL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return nil, flagutil.WithCLIValidation(fmt.Errorf("invalid server URL %q for Agent Platform models", serverURL))
	}
	s.baseURL = u.Scheme + "://" + u.Host

	// Agent Platform は OAuth のアクセストークンだけを受け付ける。
	accessToken, _ := config.ResolveRequestSecurityCredential(cmd, "access-token")
	if accessToken == "" {
		return nil, flagutil.WithCLIValidation(fmt.Errorf("--backend vertex needs an access token: run `gcloud auth application-default login` or pass --access-token"))
	}
	s.accessToken = accessToken
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
	s.client = client.WrapClientForDiagnostics(cmd, httpClient)
	return s, nil
}

// getJSON は GET して JSON を out に読む。--dry-run では送らずに示すだけなので読まない。
func (s *session) getJSON(ctx context.Context, p string, query url.Values, out any) error {
	full := s.baseURL + p
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", hooks.UserAgent(sdk.New().SDKVersion))
	req.Header.Set("Authorization", "Bearer "+s.accessToken)
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
		body, _ := io.ReadAll(res.Body)
		return sdkerrors.NewSDKDefaultError("API error occurred", res.StatusCode, string(body), res)
	}
	if s.dryRun {
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

func strPtr(s string) *string { return &s }
