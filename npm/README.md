# @breezewish/run9-cli

Install:

```sh
npm install -g @breezewish/run9-cli
```

```sh
bun install -g @breezewish/run9-cli
```

Run:

```sh
run9 auth login \
  --endpoint https://api.run9.example.com \
  --ak ak-... \
  --sk sk-...
```

Supported platforms (via bundled native binaries):

- macOS: Apple Silicon + Intel
- Linux: x86_64 + arm64

Build from source:

```sh
go build -o ./bin/run9 ./cmd/run9
./bin/run9
```
