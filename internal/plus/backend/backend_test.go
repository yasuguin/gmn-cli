package backend

import "testing"

func TestHostとServerURL(t *testing.T) {
	cases := []struct {
		location, host, url string
	}{
		{"global", "aiplatform.googleapis.com", "https://aiplatform.googleapis.com/v1beta1/projects/p/locations"},
		{"us-central1", "us-central1-aiplatform.googleapis.com", "https://us-central1-aiplatform.googleapis.com/v1beta1/projects/p/locations"},
	}
	for _, c := range cases {
		s := Settings{Backend: Vertex, Project: "p", Location: c.location}
		if s.Host() != c.host || s.ServerURL() != c.url {
			t.Errorf("%s: host=%s url=%s", c.location, s.Host(), s.ServerURL())
		}
	}
}

func TestUSE_VERTEXAI環境変数の解釈(t *testing.T) {
	for in, want := range map[string]string{"true": Vertex, "TRUE": Vertex, "1": Vertex, "false": Gemini, "0": Gemini, "": "", "yes": ""} {
		if got := parseUseVertexEnv(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}
