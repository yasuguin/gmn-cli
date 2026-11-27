#!/usr/bin/env bash
#
# gmn（fork）のインストールスクリプト。本家の scripts/install.sh を土台にし、次を変えている。
#   - 取得元は fork の GitHub Releases（タグ gmn-v<版>）
#   - checksums.txt で照合し、一致しなければ何も入れずに止まる
#   - 入れるファイル名は gmn。公式の gemini-api は上書きしない
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/yasuguin/gmn-cli/main/scripts/gmn/install.sh | bash
#
# Options:
#   GMN_INSTALL_DIR - 入れ先（既定: 書けるなら /usr/local/bin、だめなら ~/.local/bin）
#   GMN_VERSION     - 版（例 0.1.0 または gmn-v0.1.0）。既定は latest
#
# Windows は対象外。zip を Releases から手で取る。

set -euo pipefail

REPO="yasuguin/gmn-cli"
TAG_PREFIX="gmn-v"
BINARY_NAME="gmn"
DEFAULT_INSTALL_DIR="/usr/local/bin"
USER_INSTALL_DIR="$HOME/.local/bin"
VERSION="${GMN_VERSION:-latest}"
# GMN_DOWNLOAD_BASE と GMN_PLATFORM は scripts/gmn/install_test.sh が
# ローカルの fixture を指すために使う。通常の利用では設定しない（既定は uname から決める）。
DOWNLOAD_BASE="${GMN_DOWNLOAD_BASE:-https://github.com/${REPO}/releases/download}"

log_info() { echo "[INFO] $1"; }
log_error() { echo "[ERROR] $1" >&2; }

detect_platform() {
  if [ -n "${GMN_PLATFORM:-}" ]; then
    echo "$GMN_PLATFORM"
    return
  fi
  local os arch
  case "$(uname -s)" in
    Linux*) os="Linux" ;;
    Darwin*) os="Darwin" ;;
    *)
      log_error "未対応の OS: $(uname -s)（Windows は zip を Releases から手で取る）"
      exit 1
      ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) arch="x86_64" ;;
    aarch64 | arm64) arch="arm64" ;;
    *)
      log_error "未対応の arch: $(uname -m)"
      exit 1
      ;;
  esac
  echo "${os}_${arch}"
}

resolve_tag() {
  if [ "$VERSION" = "latest" ]; then
    local latest tag
    latest="https://api.github.com/repos/${REPO}/releases/latest"
    tag=$(curl -fsSL "$latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/') || tag=""
    if [ -z "$tag" ]; then
      log_error "最新リリースの版を取れなかった（GMN_VERSION で版を指定すれば API を使わない）"
      exit 1
    fi
    echo "$tag"
    return
  fi
  case "$VERSION" in
    "$TAG_PREFIX"*) echo "$VERSION" ;;
    *) echo "${TAG_PREFIX}${VERSION}" ;;
  esac
}

get_install_dir() {
  if [ -n "${GMN_INSTALL_DIR:-}" ]; then
    echo "$GMN_INSTALL_DIR"
    return
  fi
  if [ -w "$DEFAULT_INSTALL_DIR" ] || [ -w "$(dirname "$DEFAULT_INSTALL_DIR")" ]; then
    echo "$DEFAULT_INSTALL_DIR"
    return
  fi
  log_info "$DEFAULT_INSTALL_DIR に書けないので $USER_INSTALL_DIR に入れる" >&2
  echo "$USER_INSTALL_DIR"
}

download() {
  local url="$1" out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$out"
  else
    log_error "curl か wget が必要"
    exit 1
  fi
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

main() {
  # EXIT の trap は関数の外で動くので、tmp_dir は local にしない。
  local platform tag archive base install_dir expected actual
  platform="$(detect_platform)"
  tag="$(resolve_tag)"
  archive="gmn-cli_${platform}.tar.gz"
  base="${DOWNLOAD_BASE}/${tag}"
  install_dir="$(get_install_dir)"

  log_info "版: ${tag#"$TAG_PREFIX"}（タグ ${tag}）／ 対象: ${platform} ／ 入れ先: ${install_dir}"

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf -- "$tmp_dir"' EXIT

  download "${base}/${archive}" "$tmp_dir/$archive" || {
    log_error "アーカイブを取れなかった: ${base}/${archive}"
    exit 1
  }
  download "${base}/checksums.txt" "$tmp_dir/checksums.txt" || {
    log_error "checksums.txt を取れなかった: ${base}/checksums.txt"
    exit 1
  }

  # 照合できないものは入れない。checksums.txt の行は「<hash>  <ファイル名>」の形。
  expected="$(awk -v f="$archive" '$2 == f {print $1}' "$tmp_dir/checksums.txt")"
  if [ -z "$expected" ]; then
    log_error "checksums.txt に ${archive} の行が無い。入れずに止まる"
    exit 1
  fi
  actual="$(sha256_of "$tmp_dir/$archive")"
  if [ "$actual" != "$expected" ]; then
    log_error "checksum が一致しない（expected ${expected}, actual ${actual}）。入れずに止まる"
    exit 1
  fi
  log_info "checksum を確認した"

  tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"
  if [ ! -f "$tmp_dir/$BINARY_NAME" ]; then
    log_error "アーカイブの中に ${BINARY_NAME} が無い"
    exit 1
  fi

  mkdir -p "$install_dir"
  mv "$tmp_dir/$BINARY_NAME" "$install_dir/$BINARY_NAME"
  chmod +x "$install_dir/$BINARY_NAME"
  log_info "${install_dir}/${BINARY_NAME} に入れた"

  if ! command -v "$BINARY_NAME" >/dev/null 2>&1; then
    log_info "PATH に ${install_dir} が無い。~/.zshrc か ~/.bashrc に 'export PATH=\"\$PATH:${install_dir}\"' を足す"
  fi
}

main "$@"
