# run9-cli

## Getting Started

```shell
bun install -g @breezewish/run9-cli
# or: npm install -g @breezewish/run9-cli

run9 auth login \
  --endpoint https://api.run9.example.com \
  --ak ... \
  --sk ...

run9 snap import public.ecr.aws/docker/library/alpine:3.20

run9 box create

run9 box create my-box

run9 box create my-box --description "My workspace" --shape 2c4g --image public.ecr.aws/docker/library/alpine:3.20

run9 box exec my-box /bin/sh -lc 'echo hello'

run9 box cp ./local.txt my-box:/work/local.txt

run9 box stop my-box
run9 box commit my-box

```

## Supported Platforms

- macOS: Apple Silicon + Intel
- Linux: x86_64 + arm64

## Development

```bash
go build -o run9 ./cmd/run9
```

**Release:**

- Create a git tag named `vX.Y.Z` or `vX.Y.Z-<prerelease>` and push to GitHub
- GitHub Actions will automatically build binaries for 4 platforms, create a GitHub Release, and publish the npm package `@breezewish/run9-cli`
