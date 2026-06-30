# ECS Benchmark

一个用于容器压测/基准测试的 Go HTTP 服务。它接收 HTTP 请求后，在当前进程内制造可控的 CPU 和内存使用量，适合部署到 ECS、Docker、Kubernetes 等容器环境中观察调度、扩缩容、监控和告警行为。

## API

启动后默认监听 `:8080`。

```bash
curl localhost:8080/healthz
```

设置负载：

```bash
curl -X POST localhost:8080/load \
  -H 'Content-Type: application/json' \
  -d '{"cpu_percent":50,"memory_mb":256}'
```

查看当前设置：

```bash
curl localhost:8080/load
```

重置负载：

```bash
curl -X POST localhost:8080/reset
```

## 参数

`cpu_percent`：0 到 100。服务会按 `WORKERS` 个 worker 近似制造该比例的 CPU 忙等负载。

`memory_mb`：保留的内存大小，单位 MiB。服务会实际触碰分配的页面，让 RSS 更容易体现出来。

## 环境变量

`ADDR`：监听地址，默认 `:8080`。

`WORKERS`：CPU worker 数，默认使用 Go 的 `GOMAXPROCS`。

`MAX_MEMORY_MB`：允许请求的最大内存，默认 `4096`。

## 本地运行

```bash
go test ./...
go run -buildvcs=false ./cmd/ecs-benchmark
```

## 构建容器镜像

```bash
docker build -t ecs-benchmark:latest .
docker run --rm -p 8080:8080 ecs-benchmark:latest
```
