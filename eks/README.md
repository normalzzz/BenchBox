# BenchBox on EKS

这些清单用于把 BenchBox 部署到 EKS。

## 部署

先把镜像推到 ECR，然后修改 `deployment.yaml` 中的镜像地址：

```yaml
image: <account-id>.dkr.ecr.<region>.amazonaws.com/benchbox:latest
```

应用清单：

```bash
kubectl apply -k eks
```

查看资源：

```bash
kubectl -n benchbox get pods
kubectl -n benchbox get svc benchbox
```

访问服务：

```bash
curl http://<load-balancer-dns>/healthz
curl -X POST http://<load-balancer-dns>/load \
  -H 'Content-Type: application/json' \
  -d '{"cpu_percent":50,"memory_mb":256}'
```

## 说明

`WORKERS=4` 时，`cpu_percent=50` 大约会制造 4 个 worker 各 50% 的 CPU 负载，也就是接近 2 个 CPU core。

`MAX_MEMORY_MB=3072` 与 Deployment 的 `memory: 3Gi` limit 对齐，避免请求过大内存导致 Pod 被 OOMKilled。
