#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <version>" >&2
  exit 1
fi

version="$1"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd "${script_dir}/.." && pwd)"
dist_root="${project_root}/dist"

source "${script_dir}/release-targets.sh"

stage_root="$(mktemp -d "${TMPDIR:-/tmp}/run9-cli-npm-stage.XXXXXX")"
cleanup() {
  rm -rf "${stage_root}"
}
trap cleanup EXIT

mkdir -p "${stage_root}/bin"
mkdir -p "${stage_root}/vendor"

cp -R "${project_root}/npm/bin/." "${stage_root}/bin/"
cp "${project_root}/npm/README.md" "${stage_root}/README.md"
cp "${project_root}/npm/package.json" "${stage_root}/package.json"

node -e "
  const fs = require('fs');
  const packagePath = process.argv[1];
  const version = process.argv[2];
  const pkg = JSON.parse(fs.readFileSync(packagePath, 'utf8'));
  pkg.version = version;
  fs.writeFileSync(packagePath, JSON.stringify(pkg, null, 2) + '\n');
" "${stage_root}/package.json" "${version}"

for target in "${RUN9_RELEASE_TARGETS[@]}"; do
  binary_path="${dist_root}/${target}/run9"
  [[ -f "${binary_path}" ]] \
    || { echo "missing run9 binary for target ${target} at ${binary_path}" >&2; exit 1; }

  vendor_target_dir="${stage_root}/vendor/${target}/run9"
  mkdir -p "${vendor_target_dir}"
  cp "${binary_path}" "${vendor_target_dir}/run9"
  chmod +x "${vendor_target_dir}/run9"
done

mkdir -p "${dist_root}/npm"

tarball_name="$(
  cd "${stage_root}" \
    && npm pack --json --pack-destination "${dist_root}/npm" \
    | node -e "const fs=require('fs'); const out=JSON.parse(fs.readFileSync(0,'utf8')); process.stdout.write(out[0].filename || out[0].name);"
)"

mv -f "${dist_root}/npm/${tarball_name}" "${dist_root}/npm/run9-cli-npm-${version}.tgz"
printf '%s\n' "${dist_root}/npm/run9-cli-npm-${version}.tgz"
