package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type fakeTransport struct{ got []http.Header }

func (f *fakeTransport) Do(req *http.Request) (*http.Response, error) {
	f.got = append(f.got, req.Header.Clone())
	return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
}

// counterSource は呼ばれるたびに別のトークンを返す（期限切れ後の取り直しの代わり）。
type counterSource struct {
	mu sync.Mutex
	n  int
}

func (c *counterSource) Token() (*oauth2.Token, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return &oauth2.Token{AccessToken: fmt.Sprintf("tok-%d", c.n)}, nil
}

func do(t *testing.T, c interface {
	Do(*http.Request) (*http.Response, error)
}, setup func(*http.Request)) {
	t.Helper()
	req := httptest.NewRequest("POST", "http://example.test/x", nil)
	if setup != nil {
		setup(req)
	}
	if _, err := c.Do(req); err != nil {
		t.Fatal(err)
	}
}

func TestリクエストごとにTokenSourceから取り直してBearerを付ける(t *testing.T) {
	inner := &fakeTransport{}
	c := &adcClient{next: inner, load: func(context.Context) (oauth2.TokenSource, error) { return &counterSource{}, nil }}
	do(t, c, nil)
	do(t, c, nil)
	if len(inner.got) != 2 || inner.got[0].Get("Authorization") != "Bearer tok-1" || inner.got[1].Get("Authorization") != "Bearer tok-2" {
		t.Errorf("headers = %v", inner.got)
	}
}

func TestAuthorizationが既にあれば上書きしない(t *testing.T) {
	inner := &fakeTransport{}
	src := &counterSource{}
	c := &adcClient{next: inner, load: func(context.Context) (oauth2.TokenSource, error) { return src, nil }}
	do(t, c, func(r *http.Request) { r.Header.Set("Authorization", "Bearer mine") })
	if inner.got[0].Get("Authorization") != "Bearer mine" || src.n != 0 {
		t.Errorf("headers = %v, token calls = %d", inner.got, src.n)
	}
}

func TestDryRunでは資格情報を読まず伏せたトークンを付ける(t *testing.T) {
	inner := &fakeTransport{}
	c := &adcClient{next: inner, dryRun: true, load: func(context.Context) (oauth2.TokenSource, error) {
		t.Fatal("dry-run で ADC を読んではいけない")
		return nil, nil
	}}
	do(t, c, nil)
	if inner.got[0].Get("Authorization") == "" {
		t.Error("dry-run のプレビューに Authorization が無い")
	}
}

func TestADCの読み込みは1回だけ(t *testing.T) {
	inner := &fakeTransport{}
	loads := 0
	c := &adcClient{next: inner, load: func(context.Context) (oauth2.TokenSource, error) {
		loads++
		return &counterSource{}, nil
	}}
	do(t, c, nil)
	do(t, c, nil)
	if loads != 1 {
		t.Errorf("loads = %d", loads)
	}
}

// expiringSource は有効期限つきのトークンを返す。ReuseTokenSource 越しに、期限切れなら取り直し、期限内なら使い回すことを確かめる。
type expiringSource struct {
	n      int
	expiry func(n int) time.Time
}

func (e *expiringSource) Token() (*oauth2.Token, error) {
	e.n++
	return &oauth2.Token{AccessToken: fmt.Sprintf("tok-%d", e.n), Expiry: e.expiry(e.n)}, nil
}

func TestADCは期限切れのトークンを次のリクエストで取り直し期限内なら使い回す(t *testing.T) {
	for name, c := range map[string]struct {
		expiry func(int) time.Time
		want   []string
	}{
		"期限切れ": {func(int) time.Time { return time.Now().Add(-time.Minute) }, []string{"Bearer tok-1", "Bearer tok-2", "Bearer tok-3"}},
		"期限内":  {func(int) time.Time { return time.Now().Add(time.Hour) }, []string{"Bearer tok-1", "Bearer tok-1", "Bearer tok-1"}},
	} {
		t.Run(name, func(t *testing.T) {
			inner := &fakeTransport{}
			src := oauth2.ReuseTokenSource(nil, &expiringSource{expiry: c.expiry})
			ad := &adcClient{next: inner, load: func(context.Context) (oauth2.TokenSource, error) { return src, nil }}
			for range c.want {
				do(t, ad, nil)
			}
			for i, want := range c.want {
				if got := inner.got[i].Get("Authorization"); got != want {
					t.Errorf("%d 回目の Authorization = %q, want %q", i+1, got, want)
				}
			}
		})
	}
}

func TestキャンセルされたcontextのリクエストでTokenSourceを作っても後のリクエストが成功する(t *testing.T) {
	inner := &fakeTransport{}
	// 渡された context が生きている間だけトークンを返す（oauth2 が context を更新に使うのを模す）。
	load := func(ctx context.Context) (oauth2.TokenSource, error) {
		return ctxSource{ctx}, nil
	}
	c := &adcClient{next: inner, load: load}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("POST", "http://example.test/x", nil).WithContext(ctx)
	if _, err := c.Do(req); err != nil {
		t.Fatal(err)
	}
	cancel()
	do(t, c, nil)
}

func TestADCの読み出しに失敗してもキャッシュせず次のリクエストで再試行する(t *testing.T) {
	inner := &fakeTransport{}
	calls := 0
	load := func(context.Context) (oauth2.TokenSource, error) {
		calls++
		if calls == 1 {
			return nil, fmt.Errorf("一時的な失敗")
		}
		return &counterSource{}, nil
	}
	c := &adcClient{next: inner, load: load}
	if _, err := c.Do(httptest.NewRequest("POST", "http://example.test/x", nil)); err == nil {
		t.Fatal("1 回目は失敗するはず")
	}
	do(t, c, nil)
}

type ctxSource struct{ ctx context.Context }

func (s ctxSource) Token() (*oauth2.Token, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	return &oauth2.Token{AccessToken: "tok"}, nil
}
