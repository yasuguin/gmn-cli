#!/usr/bin/env bash
# Agent Platform へ実際に送る smoke スクリプト。保守者が手元で回す（CI には置かない）。
# 主要コマンドを 1 本ずつ実行し、チェックごとに PASS・FAIL・SKIP を 1 行で出して最後に集計する。
# 個々の失敗で止まらず最後まで回す。ログ全文は一時ディレクトリに残す。
#
# 費用の目安: --probe-models を付けない通常実行は数回の呼び出しだけ。--probe-models はテキスト 10 回・画像 5 回・
# TTS 4 回で、数十円〜百円程度の見込み（料金表の一次情報は未確認の概算）。
#
# 使い方は scripts/gmn/live-smoke.sh --help を見る。
set -uo pipefail

usage() {
  cat <<'EOF'
Agent Platform へ実際に送る smoke スクリプト（保守者が手元で回す。CI には置かない）

使い方:
  GOOGLE_CLOUD_PROJECT=<your-project> scripts/gmn/live-smoke.sh
  GOOGLE_CLOUD_PROJECT=<your-project> scripts/gmn/live-smoke.sh --probe-models
  SMOKE_DRY_RUN=1 GOOGLE_CLOUD_PROJECT=dummy scripts/gmn/live-smoke.sh

必要な環境変数:
  GOOGLE_CLOUD_PROJECT   必須。課金先の project（出力とログには伏せて <project> と出す）
  GOOGLE_CLOUD_LOCATION  任意。既定は global
  SMOKE_GCS_URI          任意。gs:// の URI（例 gs://BUCKET/sample.mp4）。あれば analyze と（音声なら）transcribe を送る
  SMOKE_VIDEO=1          任意。video を 1 本（360p）送る。費用がかかるので既定では回さない
  SMOKE_DRY_RUN=1        任意。通信しない。各コマンドに --dry-run を付けて流れだけ確かめる（ADC も読まない）
  SMOKE_PROBE_API_VERSION 任意。--probe-models の generateContent の API バージョン（既定 v1）

認証: gcloud auth application-default login 済みの ADC を使う（API key は使わない）。

フラグ:
  --probe-models  モデルの調査を回す。Interactions で未確認のテキストモデル、generateContent で画像・TTS の候補を叩き、
                  応答の形（画像・音声の入るキーと mimeType）を一時ディレクトリに保存して probe-summary.txt を出す。
                  既定では回さない。
  -h, --help      この説明を出す

費用の目安:
  通常実行は数回の呼び出し。--probe-models は テキスト 10 回・画像 5 回・TTS 4 回で数十円〜百円程度の見込み（料金表は未確認）。

出力:
  PASS|FAIL|SKIP <name> exit=<code> <要点>。最後に件数を集計し、ログの場所を出す。
  FAIL が 1 つでもあると終了コードは 1。
EOF
}

probe=0
for arg in "$@"; do
  case "$arg" in
    -h | --help)
      usage
      exit 0
      ;;
    --probe-models) probe=1 ;;
    *)
      echo "不明な引数: $arg" >&2
      usage >&2
      exit 2
      ;;
  esac
done

project="${GOOGLE_CLOUD_PROJECT:-}"
if [ -z "$project" ]; then
  echo "GOOGLE_CLOUD_PROJECT が無い。--help を見て設定する" >&2
  exit 2
fi
# 形が合わない値はログにも出さないよう、中身を表示せずに止める。
if ! printf '%s' "$project" | grep -Eq '^[a-z0-9][a-z0-9:.-]*$'; then
  echo "GOOGLE_CLOUD_PROJECT の形が project ID と合わない" >&2
  exit 2
fi
location="${GOOGLE_CLOUD_LOCATION:-global}"
if ! printf '%s' "$location" | grep -Eq '^[a-z0-9-]+$'; then
  echo "GOOGLE_CLOUD_LOCATION の形が合わない" >&2
  exit 2
fi
dry=0
if [ "${SMOKE_DRY_RUN:-}" = 1 ]; then
  dry=1
fi

root="$(cd "$(dirname "$0")/../.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/gmn-smoke.XXXXXX")"
logdir="$work/logs"
probedir="$work/probe"
assets="$work/assets"
out_dir="$work/out"
bin_dir="$work/bin"
mkdir -p "$logdir" "$probedir" "$assets" "$out_dir" "$bin_dir"
bin="$bin_dir/gmn"

real_home="$HOME"

# ビルドは install せず一時ディレクトリへ出す。ビルドだけは実ホームのまま回す（Go のキャッシュ用）。
if ! (cd "$root" && go build -o "$bin" ./cmd/gmn) >"$logdir/build.log" 2>&1; then
  echo "ビルドに失敗した。ログ: $logdir/build.log" >&2
  exit 1
fi

# HOME を一時ディレクトリへ向けて repo の設定や実ホームの config を読ませない。
# golang.org/x/oauth2/google は Unix で ADC を $HOME/.config/gcloud からしか探さず、CLOUDSDK_CONFIG を見ない。
# そのため GOOGLE_APPLICATION_CREDENTIALS が無いときは、HOME を替える前に実ホームの ADC ファイルをそれで渡す。
export CLOUDSDK_CONFIG="${CLOUDSDK_CONFIG:-$real_home/.config/gcloud}"
adc_file="$CLOUDSDK_CONFIG/application_default_credentials.json"
if [ -z "${GOOGLE_APPLICATION_CREDENTIALS:-}" ] && [ -f "$adc_file" ]; then
  export GOOGLE_APPLICATION_CREDENTIALS="$adc_file"
fi
export HOME="$work/home"
mkdir -p "$HOME"
unset GEMINI_API_KEY GOOGLE_API_KEY GEMINI_ACCESS_TOKEN
export GOOGLE_GENAI_USE_VERTEXAI=true
export GOOGLE_CLOUD_PROJECT="$project"
export GOOGLE_CLOUD_LOCATION="$location"

# --dry-run のときは通信しない。common は必ず 1 要素以上にする（macOS の bash 3.2 は空配列の展開で止まるため）。
if [ "$dry" = 1 ]; then
  common=(--no-keyring --no-interactive --dry-run)
else
  common=(--no-keyring --no-interactive)
fi

# ログと表示に project と実ホームを出さない。work は場所を出すので伏せない。
mask_text() {
  printf '%s' "$1" | sed -e "s|$project|<project>|g" -e "s|$real_home|<home>|g"
}

mask_file() {
  [ -s "$1" ] || return 0
  sed -e "s|$project|<project>|g" -e "s|$real_home|<home>|g" "$1" >"$1.tmp" && mv "$1.tmp" "$1"
}

first_line() {
  grep -m1 -v '^[[:space:]]*$' "$1" 2>/dev/null | LC_CTYPE=UTF-8 cut -c1-80
}

clip() {
  printf '%s' "$1" | LC_CTYPE=UTF-8 cut -c1-80
}

# テスト素材はスクリプトが作る。macOS に無いときは該当チェックを SKIP にする。
printf 'Agent Platform smoke test\n' >"$assets/sample.txt"
jpg=""
wav=""
if command -v sips >/dev/null 2>&1; then
  src="$(find "/System/Library/Desktop Pictures" -maxdepth 1 \( -name '*.heic' -o -name '*.jpg' -o -name '*.png' \) 2>/dev/null | head -1)"
  if [ -n "$src" ] && sips -s format jpeg -Z 512 "$src" --out "$assets/sample.jpg" >/dev/null 2>&1; then
    jpg="$assets/sample.jpg"
  fi
fi
if command -v say >/dev/null 2>&1; then
  if say -v Samantha -o "$assets/sample.wav" --data-format=LEI16@16000 "Hello from Agent Platform" >/dev/null 2>&1 ||
    say -o "$assets/sample.wav" --data-format=LEI16@16000 "Hello from Agent Platform" >/dev/null 2>&1; then
    wav="$assets/sample.wav"
  fi
fi

pass=0
fail=0
skip=0
seq=0
LAST_OUT=""

# run_check NAME 想定 exit PATTERN 引数...
# PATTERN は標準出力と標準エラーを 1 行につないだものへの ERE（空なら終了コードだけ見る）。dry-run では PATTERN を見ない。
run_check() {
  local name="$1" want="$2" pat="$3"
  shift 3
  seq=$((seq + 1))
  local base
  base="$(printf '%s/%02d-%s' "$logdir" "$seq" "$name")"
  "$bin" "$@" "${common[@]}" >"$base.out" 2>"$base.err"
  local rc=$?
  LAST_OUT="$base.out"
  mask_file "$base.out"
  mask_file "$base.err"
  local combined
  combined="$(cat "$base.out" "$base.err" | tr '\n' ' ')"
  local ok=0
  if [ "$rc" -eq "$want" ]; then
    if [ "$dry" = 1 ] || [ -z "$pat" ] || printf '%s' "$combined" | grep -Eiq -- "$pat"; then
      ok=1
    fi
  fi
  local point=""
  if [ -n "$pat" ]; then
    point="$(cat "$base.out" "$base.err" | grep -Ei -m1 -- "$pat")"
  fi
  if [ -z "$point" ]; then
    point="$(first_line "$base.out")"
  fi
  if [ -z "$point" ]; then
    point="$(first_line "$base.err")"
  fi
  if [ "$ok" = 1 ]; then
    pass=$((pass + 1))
    printf 'PASS %s exit=%s %s\n' "$name" "$rc" "$(clip "$(mask_text "$point")")"
  else
    fail=$((fail + 1))
    local why="出力が想定と違う"
    if [ "$rc" -ne "$want" ]; then
      why="想定 exit=$want"
    fi
    if [ "$rc" -ne "$want" ]; then
      point="$(first_line "$base.err")"
      [ -z "$point" ] && point="$(first_line "$base.out")"
    fi
    printf 'FAIL %s exit=%s %s\n' "$name" "$rc" "$(clip "$why: $(mask_text "$point")")"
  fi
}

# file_check NAME PATH。直前のコマンドが書き出したファイルが空でなく存在することを PASS・FAIL にする。
file_check() {
  if [ "$dry" = 1 ] || [ -s "$2" ]; then
    pass=$((pass + 1))
    printf 'PASS %s exit=- %s\n' "$1" "$(clip "$(basename "$2")")"
  else
    fail=$((fail + 1))
    printf 'FAIL %s exit=- 画像ファイルが書き出されていない: %s\n' "$1" "$(basename "$2")"
  fi
}

# wav_check NAME PATH。書き出したファイルが WAV（先頭が RIFF）であることを PASS・FAIL にする。
wav_check() {
  if [ "$dry" = 1 ] || [ "$(head -c 4 "$2" 2>/dev/null)" = RIFF ]; then
    pass=$((pass + 1))
    printf 'PASS %s exit=- %s\n' "$1" "$(clip "$(basename "$2")")"
  else
    fail=$((fail + 1))
    printf 'FAIL %s exit=- WAV として書き出されていない: %s\n' "$1" "$(basename "$2")"
  fi
}

skip_check() {
  skip=$((skip + 1))
  printf 'SKIP %s exit=- %s\n' "$1" "$2"
}

interaction_id() {
  if command -v jq >/dev/null 2>&1; then
    jq -r '.id // empty' "$1" 2>/dev/null | head -1
  else
    sed -n 's/.*"id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$1" | head -1
  fi
}

run_check whoami 0 'vertex.*adc' whoami

# -o json で受けて interaction id を取り、agent status に渡す。
run_check generate 0 '"id"' generate "Say hi in 3 words" --stream=false -o json
gen_out="$LAST_OUT"
if [ "$dry" = 1 ]; then
  gen_id="dry-run-interaction"
else
  gen_id="$(interaction_id "$gen_out")"
fi

if [ -n "$jpg" ]; then
  run_check analyze-image 0 '[^[:space:]]' analyze -i "$jpg" "one sentence"
else
  skip_check analyze-image "jpg の素材が作れない（sips が無い）"
fi

if [ -n "$wav" ]; then
  # 既定モデルは modelmap の読み替えで gemini-3.8-flash になり、その旨が stderr に出る（-o json・--agent-mode では出ない）。
  run_check transcribe-default 0 'Using gemini-3.8-flash: gemini-3.5-transcribe is not available on Agent Platform' transcribe -i "$wav" --out "$out_dir/transcript.md"
  run_check transcribe-unsupported-model 2 'not available' transcribe -i "$wav" --model gemini-3.5-transcribe
else
  skip_check transcribe-default "wav の素材が作れない（say が無い）"
  skip_check transcribe-unsupported-model "wav の素材が作れない（say が無い）"
fi

run_check models-list-count 0 '^[1-9][0-9]*' models list --jq '[.models[].name] | length'
run_check models-get 0 'gemini-4-hydrogen' models get gemini-4-hydrogen
run_check agent-list 0 '' agent list
if [ -n "$gen_id" ]; then
  run_check agent-status 0 '' agent status "$gen_id"
  run_check agent-delete-interaction 0 '' agent delete-interaction "$gen_id"
else
  skip_check agent-status "generate の応答から interaction id が取れなかった"
  skip_check agent-delete-interaction "generate の応答から interaction id が取れなかった"
fi

run_check environments-unsupported 2 'not available|unknown command' environments list
run_check files-upload-unsupported 2 'gs://' files upload "$assets/sample.txt"

if [ -n "${SMOKE_GCS_URI:-}" ]; then
  if printf '%s' "$SMOKE_GCS_URI" | grep -Eq '^gs://[^[:space:]]+$'; then
    run_check analyze-gcs 0 '[^[:space:]]' analyze -i "$SMOKE_GCS_URI" "describe"
    case "$(printf '%s' "$SMOKE_GCS_URI" | tr '[:upper:]' '[:lower:]')" in
      *.wav | *.mp3 | *.m4a | *.flac | *.ogg | *.aac | *.aiff)
        run_check transcribe-gcs 0 '[^[:space:]]' transcribe -i "$SMOKE_GCS_URI" --out "$out_dir/transcript-gcs.md"
        ;;
      *)
        skip_check transcribe-gcs "音声の URI ではない"
        ;;
    esac
  else
    fail=$((fail + 1))
    printf 'FAIL gcs-uri exit=- SMOKE_GCS_URI は gs:// で始まる URI にする\n'
  fi
else
  skip_check analyze-gcs "SMOKE_GCS_URI が無い"
  skip_check transcribe-gcs "SMOKE_GCS_URI が無い"
fi

# image は generateContent で呼び、最終画像（thought でないもの）が書き出されることまで確かめる。
# tts も generateContent で呼び、書き出した WAV の先頭が RIFF であることまで確かめる。
run_check image 0 '' image "a small red apple on white background" --out "$out_dir/image.png"
file_check image-file "$out_dir/image.png"
run_check tts 0 '' tts "Hello from Agent Platform" --out "$out_dir/tts.wav"
wav_check tts-file "$out_dir/tts.wav"

# music は response_format を付けずに送り、既定の lyria-3.5 を lyria-3-pro-preview に替える。
# 素朴なプロンプトは recitation（既存の曲に似すぎる）で断られることがあるので、具体的に書く。
run_check music 0 'Using lyria-3-pro-preview' music "a short original lo-fi beat with soft vinyl crackle, mellow electric piano in D minor, 80 bpm" --out "$out_dir/music.mp3"
file_check music-file "$out_dir/music.mp3"
run_check music-unsupported-model 2 'lyria-3-pro-preview' music "calm piano" -m lyria-3.5 --out "$out_dir/music-unsupported.mp3"

if [ "${SMOKE_VIDEO:-}" = 1 ]; then
  run_check video 0 '' video "a red ball bouncing once on a white floor" --resolution 360p --out "$out_dir/video.mp4"
  file_check video-file "$out_dir/video.mp4"
else
  skip_check video "SMOKE_VIDEO=1 が無い（費用がかかるので既定では回さない）"
fi

# Interactions API のマルチリージョン（送り先は aiplatform.<location>.rep.googleapis.com）。
run_check generate-us 0 '' generate "Say hi in 3 words" --location us --stream=false
run_check generate-eu 0 '' generate "Say hi in 3 words" --location eu --stream=false

probe_text_models=(gemini-2.5-flash gemini-2.5-pro gemini-3.5-flash gemini-3.7-flash gemini-4-argon gemini-4-carbon gemini-4-neon gemini-flash-latest gemini-pro-latest gemma-4-31b-it)
probe_image_models=(gemini-nano-banana-2.1 gemini-3.1-flash-image gemini-3-pro-image gemini-2.5-flash-image nano-banana-pro-preview)
probe_tts_models=(gemini-3.8-flash-tts gemini-3.8-flash-lite-tts gemini-3.1-flash-tts-preview gemini-2.5-flash-preview-tts)

summary="$probedir/probe-summary.txt"

probe_line() {
  local line
  line="$(mask_text "$1")"
  printf '%s\n' "$line"
  printf '%s\n' "$line" >>"$summary"
}

# Interactions 経由で、表に無いテキストモデルが通るかを 1 回ずつ見る。
probe_text() {
  local m="$1" f="$probedir/text-$m"
  "$bin" generate "hi" --model "$m" --stream=false "${common[@]}" >"$f.out" 2>"$f.err"
  local rc=$?
  mask_file "$f.out"
  mask_file "$f.err"
  if [ "$dry" = 1 ]; then
    probe_line "DRY $m exit=$rc"
  elif [ "$rc" -eq 0 ]; then
    probe_line "OK $m $(first_line "$f.out")"
  else
    probe_line "NG $m $(first_line "$f.err")"
  fi
}

# generateContent を直接叩き、画像・音声が返るかと入るキーを見る。トークンは 600 の一時ファイル経由で渡し、表示しない。
probe_content() {
  local kind="$1"
  local m="$2"
  local f="$probedir/$kind-$m"
  local ver="${SMOKE_PROBE_API_VERSION:-v1}"
  local host="aiplatform.googleapis.com"
  if [ "$location" != "global" ]; then
    host="$location-aiplatform.googleapis.com"
  fi
  local url="https://$host/$ver/projects/$project/locations/$location/publishers/google/models/$m:generateContent"
  local body="$f.request.json"
  if [ "$kind" = image ]; then
    # 画像は generateContent の公式例（responseModalities に TEXT と IMAGE）の形を使う。
    printf '%s' '{"contents":[{"role":"user","parts":[{"text":"a small red apple on white background"}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"]}}' >"$body"
  else
    # TTS は公式の generateContent 例が見つからなかったので Gemini API の形を使い、応答で確かめる。
    printf '%s' '{"contents":[{"role":"user","parts":[{"text":"Hello from Agent Platform"}]}],"generationConfig":{"responseModalities":["AUDIO"],"speechConfig":{"voiceConfig":{"prebuiltVoiceConfig":{"voiceName":"Kore"}}}}}' >"$body"
  fi
  if [ "$dry" = 1 ]; then
    probe_line "DRY $kind $m POST $url"
    return
  fi
  if ! command -v jq >/dev/null 2>&1; then
    probe_line "NG $kind $m jq が無いので応答を読めない"
    return
  fi
  local token
  token="$(gcloud auth application-default print-access-token 2>/dev/null)"
  if [ -z "$token" ]; then
    probe_line "NG $kind $m トークン取得に失敗（gcloud auth application-default login を確かめる）"
    return
  fi
  local hdr
  hdr="$(mktemp "$work/auth.XXXXXX")"
  {
    printf 'header = "Authorization: Bearer %s"\n' "$token"
    printf 'header = "x-goog-user-project: %s"\n' "$project"
    printf 'header = "Content-Type: application/json"\n'
  } >"$hdr"
  unset token
  local code crc
  code="$(curl -sS --config "$hdr" -X POST --data-binary "@$body" -o "$f.resp" -w '%{http_code}' "$url" 2>"$f.err")"
  crc=$?
  rm -f "$hdr"
  mask_file "$f.err"
  if [ "$crc" -ne 0 ]; then
    probe_line "NG $kind $m curl 失敗: $(first_line "$f.err")"
    return
  fi
  if [ "$code" != 200 ]; then
    local msg
    msg="$(jq -r '.error.message // empty' "$f.resp" 2>/dev/null | head -1)"
    probe_line "NG $kind $m http=$code $(clip "$msg")"
    return
  fi
  local mimes keypath
  mimes="$(jq -r '[.. | objects | select(has("inlineData")) | .inlineData.mimeType] | unique | join(",")' "$f.resp")"
  keypath="$(jq -r '([paths(type=="object" and has("inlineData"))][0] // []) | map(tostring) | join(".")' "$f.resp")"
  # base64 を伏せた形だけを残す。バイナリ本体は別ファイルに置く。
  jq 'walk(if type=="object" and has("inlineData") then .inlineData.data = "<base64 omitted>" else . end)' "$f.resp" >"$f.sanitized.json"
  jq -r 'first(.. | objects | select(has("inlineData")) | .inlineData.data) // empty' "$f.resp" | { base64 -D 2>/dev/null || base64 -d; } >"$f.bin" 2>/dev/null
  if [ ! -s "$f.bin" ]; then
    rm -f "$f.bin"
  fi
  local want_prefix="image/"
  if [ "$kind" = tts ]; then
    want_prefix="audio/"
  fi
  if printf '%s' "$mimes" | grep -q "$want_prefix"; then
    probe_line "OK $kind $m mime=$mimes key=$keypath"
  else
    probe_line "NG $kind $m $kind の inlineData が無い（mime=${mimes:-なし}）"
  fi
}

if [ "$probe" = 1 ]; then
  : >"$summary"
  probe_line "# Interactions（generate --model）"
  for m in "${probe_text_models[@]}"; do
    probe_text "$m"
  done
  probe_line "# generateContent（画像）"
  for m in "${probe_image_models[@]}"; do
    probe_content image "$m"
  done
  probe_line "# generateContent（TTS）"
  for m in "${probe_tts_models[@]}"; do
    probe_content tts "$m"
  done
  echo "probe の要約: $summary"
fi

echo "集計: PASS=$pass FAIL=$fail SKIP=$skip"
echo "ログ: $work"
if [ "$fail" -gt 0 ]; then
  exit 1
fi
exit 0
