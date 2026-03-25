#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <version> [target...]" >&2
  exit 1
fi

version="$1"
shift

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd "${script_dir}/.." && pwd)"
dist_root="${project_root}/dist"

source "${script_dir}/release-targets.sh"

targets=("$@")
if [[ ${#targets[@]} -eq 0 ]]; then
  targets=("${RUN9_RELEASE_TARGETS[@]}")
fi

cd "${project_root}"

for target in "${targets[@]}"; do
  go_platform="$(release_target_to_go_platform "${target}")"
  read -r goos goarch <<<"${go_platform}"

  target_dir="${dist_root}/${target}"
  rm -rf "${target_dir}"
  mkdir -p "${target_dir}"

  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -ldflags="-s -w" -o "${target_dir}/run9" ./cmd/run9

  chmod +x "${target_dir}/run9"
  tar -C "${target_dir}" -czf "${target_dir}/run9-${version}-${target}.tar.gz" run9
done
