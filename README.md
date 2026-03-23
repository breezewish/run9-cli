# run9-cli

## 快速开始

### 1. 构建

```bash
go build -o ./bin/run9 ./cmd/run9
```

### 2. 开始使用

```bash
./bin/run9 snap import public.ecr.aws/docker/library/alpine:3.20

./bin/run9 box create my-box

./bin/run9 box create my-box --shape 2c8g --image public.ecr.aws/docker/library/alpine:3.20

./bin/run9 box exec my-box /bin/sh -lc 'echo hello'

./bin/run9 box cp ./local.txt my-box:/work/local.txt

./bin/run9 box stop my-box
./bin/run9 box commit my-box
```
