# run9-cli

## 安装

```bash
npm install -g @breezewish/run9-cli
```

```bash
bun install -g @breezewish/run9-cli
```

## 快速开始

### 1. 从源码构建

```bash
go build -o ./bin/run9 ./cmd/run9
```

### 2. 登录

```bash
./bin/run9 auth login \
  --endpoint https://api.run9.example.com \
  --ak ak-... \
  --sk sk-...
```

### 3. 开始使用

```bash
./bin/run9 snap import public.ecr.aws/docker/library/alpine:3.20

./bin/run9 box create

./bin/run9 box create my-box

./bin/run9 box create my-box --description "My workspace" --shape 2c4g --image public.ecr.aws/docker/library/alpine:3.20

./bin/run9 box exec my-box /bin/sh -lc 'echo hello'

./bin/run9 box cp ./local.txt my-box:/work/local.txt

./bin/run9 box stop my-box
./bin/run9 box commit my-box
```

说明：

- `run9 box create` 可省略位置参数 `box_id`，由服务端自动分配 org 内唯一的 `{word}-{word}` 标识
- `run9 box create my-box` 默认使用 `1c2g`，且在未显式传 `--snap` / `--image` 时默认使用 `public.ecr.aws/docker/library/alpine:3.20`
- box 不再有 name；可选展示文本统一使用 `--description`
- `run9 box exec my-box <command...>` 不要求强制插入 `--`

## 支持的平台

- macOS: Apple Silicon + Intel
- Linux: x86_64 + arm64

## 发布

- 推送 `vX.Y.Z` 或 `vX.Y.Z-<prerelease>` git tag
- GitHub Actions 会自动编译 4 个平台二进制、创建 GitHub Release，并发布 npm 包 `@breezewish/run9-cli`
