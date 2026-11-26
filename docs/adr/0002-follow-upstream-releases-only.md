# 本家はリリースだけを追い、GitHub の「Sync fork」は使わない

本家（google-gemini/gemini-api-cli）への追従は、本家のリリースタグだけを、取り込みの PR（merge commit）で入れる。本家の main の未リリースの commit は追わず、GitHub の fork の画面に出る「Sync fork」（本家の main を fork の main へ直接 merge する）も使わない。fork の公開物（npm・PyPI・Homebrew）は、本家のリリースを土台にした版（`gmn --version` の base 表示）で揃える必要があり、未リリースの変更を混ぜると土台が曖昧になるため。また main は ruleset で PR 必須にしているので、直接 merge はそもそもできない。

そのため fork の画面には、本家の main より「N commits behind」と出続けることがあるが、本家の最新リリースが取り込み済みなら追従は足りている。取り込みの PR は週 1 回の workflow が GitHub App で作り（既定の `GITHUB_TOKEN` で作った PR では必須チェックが起動しない）、squash せず merge commit でマージする（本家の履歴を残し、次の取り込みの基点を崩さないため）。
