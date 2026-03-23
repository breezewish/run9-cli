# run9-cli

## 快速开始

### 1. 构建

```bash
go build -o ./bin/run9 ./cmd/run9
```

### 2. 开始使用

```bash
./bin/run9 snap import public.ecr.aws/docker/library/alpine:3.20

./bin/run9 box create --shape 2c4g --image public.ecr.aws/docker/library/alpine:3.20 --name demo

./bin/run9 box exec <box-id> -- /bin/sh -lc 'echo hello'

./bin/run9 box cp ./local.txt <box-id>:/work/local.txt

./bin/run9 box stop <box-id>
./bin/run9 box commit <box-id>
```
