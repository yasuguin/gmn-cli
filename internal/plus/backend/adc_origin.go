package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// adcEnvVar は ADC のファイルを指す環境変数。gcloud のログインより優先される。
const adcEnvVar = "GOOGLE_APPLICATION_CREDENTIALS"

// adcOrigin は ADC をどこから読むか。whoami で出す。
// 403 のときに「再ログインせよ」と案内されても、実際には GOOGLE_APPLICATION_CREDENTIALS の
// 別の資格情報（サービスアカウントの鍵など）が使われていて、ログインし直しても直らないことがある。
// 送る前にどの資格情報が使われるかを見せ、原因の取り違えを防ぐ。
type adcOrigin struct {
	Origin      string // GOOGLE_APPLICATION_CREDENTIALS・gcloud・metadata
	File        string
	Type        string // service_account・authorized_user など、ファイルの type
	Account     string // サービスアカウントの client_email
	IgnoredFile string // GOOGLE_APPLICATION_CREDENTIALS に隠れて使われない gcloud のファイル
	Err         string
}

// findADCOrigin は google.FindDefaultCredentials と同じ順で ADC の出どころを探す。
// トークンは取らず、ファイルから type と client_email だけを読む（鍵や refresh token は出さない）。
func findADCOrigin() adcOrigin {
	wellKnown := wellKnownADCFile()
	if path := os.Getenv(adcEnvVar); path != "" {
		o := adcOrigin{Origin: adcEnvVar, File: path}
		o.Type, o.Account, o.Err = readADCFile(path)
		if fileExists(wellKnown) {
			o.IgnoredFile = wellKnown
		}
		return o
	}
	if fileExists(wellKnown) {
		o := adcOrigin{Origin: "gcloud", File: wellKnown}
		o.Type, o.Account, o.Err = readADCFile(wellKnown)
		return o
	}
	return adcOrigin{Origin: "metadata"}
}

// wellKnownADCFile は gcloud auth application-default login が書くファイル。
// golang.org/x/oauth2/google の wellKnownFile と同じ場所（非公開なので写している）。
func wellKnownADCFile() string {
	const f = "application_default_credentials.json"
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "gcloud", f)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gcloud", f)
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func readADCFile(path string) (typ, account, errText string) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", "", err.Error()
	}
	var f struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal(body, &f); err != nil {
		return "", "", "not a JSON credentials file: " + err.Error()
	}
	return f.Type, f.ClientEmail, ""
}

// text は whoami の人間向けの 1 行。
func (o adcOrigin) text() string {
	switch o.Origin {
	case "metadata":
		return "ADC from the metadata server (no credentials file found; works only on Google Cloud)"
	case adcEnvVar:
		s := "ADC from " + adcEnvVar + ": " + o.File + o.detail()
		if o.IgnoredFile != "" {
			s += "; the gcloud login in " + o.IgnoredFile + " is not used"
		}
		return s
	default:
		return "ADC from " + o.File + o.detail()
	}
}

func (o adcOrigin) detail() string {
	switch {
	case o.Err != "":
		return " (cannot read: " + o.Err + ")"
	case o.Account != "":
		return " (" + o.Type + ", " + o.Account + ")"
	case o.Type != "":
		return " (" + o.Type + ")"
	}
	return ""
}

// fields は whoami の機械可読出力の backend.auth.adc。空の項目は出さない。
func (o adcOrigin) fields() map[string]any {
	m := map[string]any{"origin": o.Origin}
	for k, v := range map[string]string{"file": o.File, "type": o.Type, "account": o.Account, "ignored_file": o.IgnoredFile, "error": o.Err} {
		if v != "" {
			m[k] = v
		}
	}
	return m
}
