# run9-cli

## 快速开始

### 1. 构建

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

./bin/run9 box create my-box

./bin/run9 box create my-box --shape 2c4g --image public.ecr.aws/docker/library/alpine:3.20

./bin/run9 box exec my-box /bin/sh -lc 'echo hello'

./bin/run9 box cp ./local.txt my-box:/work/local.txt

./bin/run9 box stop my-box
./bin/run9 box commit my-box
```

说明：

- `run9 box create my-box` 默认使用 `1c2g`，且在未显式传 `--snap` / `--image` 时默认使用 `public.ecr.aws/docker/library/alpine:3.20`
- `run9 box exec my-box <command...>` 不要求强制插入 `--`
