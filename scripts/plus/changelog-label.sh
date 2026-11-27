#!/usr/bin/env bash
# PR の題（conventional commit の形）から、変更履歴の分類のラベルを 1 つ出す。
# GitHub のリリースノートの生成（.github/release.yml）がこのラベルで分類し、利用者に関係しない PR を外す。
#
#   feat           -> changelog: feat     （Features）
#   fix            -> changelog: fix      （Fixes）
#   chore(upstream)-> changelog: upstream （Upstream sync）
#   それ以外       -> changelog: skip     （ci・docs・refactor・リリース PR など。変更履歴に載せない）
#
# 使い方: scripts/plus/changelog-label.sh "<PR の題>"
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "使い方: $0 \"<PR の題>\"" >&2
  exit 2
fi
title=$1

if [[ "$title" =~ ^feat(\([^\)]*\))?!?: ]]; then
  echo "changelog: feat"
elif [[ "$title" =~ ^fix(\([^\)]*\))?!?: ]]; then
  echo "changelog: fix"
elif [[ "$title" =~ ^chore\(upstream\)!?: ]]; then
  echo "changelog: upstream"
else
  echo "changelog: skip"
fi
