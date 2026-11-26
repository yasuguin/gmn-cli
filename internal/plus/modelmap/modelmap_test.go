package modelmap

import (
	"errors"
	"testing"
)

func TestResolveはRenamedのモデルをAgentPlatformでの名前へ読み替える(t *testing.T) {
	use, replaced, err := resolve("common-name", Entry{Status: Renamed, Name: "platform-name"}, true)
	if err != nil || use != "platform-name" || !replaced {
		t.Errorf("use=%q replaced=%v err=%v, want platform-name true nil", use, replaced, err)
	}
}

func TestResolveはUnsupportedの既定を先頭の候補へ替え明示は止める(t *testing.T) {
	e := Entry{Status: Unsupported, Alternatives: []string{"alt-a", "alt-b"}}
	use, replaced, err := resolve("x", e, false)
	if err != nil || use != "alt-a" || !replaced {
		t.Errorf("既定: use=%q replaced=%v err=%v", use, replaced, err)
	}
	_, _, err = resolve("x", e, true)
	var ue *unsupportedError
	if !errors.As(err, &ue) || len(ue.alternatives) != 2 {
		t.Errorf("明示: err=%v, want unsupportedError（候補つき）", err)
	}
}

func TestResolveは未確認とverifiedとgenerateContentをそのまま送る(t *testing.T) {
	for _, s := range []Status{Verified, Unverified, GenerateContent} {
		use, replaced, err := resolve("m", Entry{Status: s}, true)
		if err != nil || use != "m" || replaced {
			t.Errorf("%s: use=%q replaced=%v err=%v", s, use, replaced, err)
		}
	}
}

func TestLookupは表に無いモデルをUnverifiedにしmodels接頭辞を外す(t *testing.T) {
	if got := Lookup("gemini-2.5-flash").Status; got != Unverified {
		t.Errorf("status = %s, want unverified", got)
	}
	if got := Lookup("models/gemini-3.5-transcribe").Status; got != Unsupported {
		t.Errorf("status = %s, want unsupported", got)
	}
}
