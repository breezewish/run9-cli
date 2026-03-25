#!/usr/bin/env bash

readonly RUN9_RELEASE_TARGETS=(
  x86_64-unknown-linux-musl
  aarch64-unknown-linux-musl
  x86_64-apple-darwin
  aarch64-apple-darwin
)

release_target_to_go_platform() {
  case "$1" in
    x86_64-unknown-linux-musl)
      printf 'linux amd64\n'
      ;;
    aarch64-unknown-linux-musl)
      printf 'linux arm64\n'
      ;;
    x86_64-apple-darwin)
      printf 'darwin amd64\n'
      ;;
    aarch64-apple-darwin)
      printf 'darwin arm64\n'
      ;;
    *)
      printf 'unsupported target: %s\n' "$1" >&2
      return 1
      ;;
  esac
}
