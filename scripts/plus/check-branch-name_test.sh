#!/usr/bin/env bash
# check-branch-name.sh の振る舞いテスト。PR のブランチ名が決まりの接頭辞で始まるときだけ通ることを確かめる。
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
pass=0
fail=0

check() {
  local branch=$1 want=$2 got=0
  bash "$here/check-branch-name.sh" "$branch" >/dev/null 2>&1 || got=$?
  if { [[ "$want" == ok ]] && [[ $got -eq 0 ]]; } || { [[ "$want" == ng ]] && [[ $got -ne 0 ]]; }; then
    pass=$((pass + 1)); echo "ok   - '${branch}' -> ${want}"
  else
    fail=$((fail + 1)); echo "FAIL - '${branch}' -> exit ${got}, want ${want}"
  fi
}

check "gmn/next-0.1.2" ok
check "gmn/branch-prefix" ok
check "release-please--branches--main--components--gmn" ok
check "upstream-sync/v0.6.0" ok
check "dependabot/go_modules/golang.org/x/net-0.40.0" ok
check "dependabot/" ng
check "plus/next-0.1.2" ng
check "dev/foo" ng
check "gmn/" ng
check "feature-x" ng
check "" ng

echo "結果: ok $pass / FAIL $fail"
[ "$fail" -eq 0 ]
