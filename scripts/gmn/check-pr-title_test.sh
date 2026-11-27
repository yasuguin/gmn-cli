#!/usr/bin/env bash
# check-pr-title.sh の振る舞いテスト。PR の題が英語の conventional commit の形のときだけ通ることを確かめる。
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
pass=0
fail=0

check() {
  local title=$1 want=$2 got=0
  bash "$here/check-pr-title.sh" "$title" >/dev/null 2>&1 || got=$?
  if { [[ "$want" == ok ]] && [[ $got -eq 0 ]]; } || { [[ "$want" == ng ]] && [[ $got -ne 0 ]]; }; then
    pass=$((pass + 1)); echo "ok   - '${title}' -> ${want}"
  else
    fail=$((fail + 1)); echo "FAIL - '${title}' -> exit ${got}, want ${want}"
  fi
}

check "fix(gmn): publish the npm package under @yasuguin/gmn-cli" ok
check "feat(gmn)!: drop the gemini-headless command" ok
check "chore(upstream): sync upstream v0.6.0" ok
check "chore(main): release gmn 0.1.2" ok
check "build(deps): bump golang.org/x/net from 0.39.0 to 0.40.0" ok
check "ci: bump actions/checkout from 6 to 7" ok
check "docs(gmn): explain the café example — with an em dash" ok
check "docs(gmn): エージェント向けの決まりを書く" ng
check "fix(gmn): use ADC（Application Default Credentials）" ng
check "Fix the README" ng
check "update: something" ng
check "fix(gmn):" ng
check "" ng

echo "結果: ok $pass / FAIL $fail"
[ "$fail" -eq 0 ]
