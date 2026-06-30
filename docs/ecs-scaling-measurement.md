# Measuring ECS Service Auto Scaling Latency

这份说明用于测量 BenchBox 发出负载请求后，到 ECS Service Auto Scaling 产生扩容动作的时间。

## 建议拆成四个时间点

| 时间点 | 含义 | 观测方式 |
| --- | --- | --- |
| `t0` | 发出 `POST /load` 的时间 | 本地脚本记录 |
| `t1` | Application Auto Scaling 产生 scaling activity | `application-autoscaling describe-scaling-activities` |
| `t2` | ECS service `desiredCount` 增加 | `ecs describe-services` |
| `t3` | 新 task 进入 RUNNING | `ecs describe-services` 的 `runningCount` 增加 |

通常报告两个关键指标：

- `t1 - t0`：负载触发自动扩缩容决策的耗时。
- `t3 - t0`：从发压到新任务真正运行起来的端到端耗时。

## 运行脚本

```bash
scripts/measure-ecs-scale.sh \
  --cluster <cluster-name> \
  --service <service-name> \
  --url http://<benchbox-load-balancer-dns> \
  --cpu 100 \
  --memory 256 \
  --region <aws-region>
```

输出示例：

```text
t0 curl_start 2026-06-30T05:00:00+00:00
t+120ms curl_completed
t+65432ms scaling_activity_seen activity_id=...
t+67001ms ecs_desired_count_changed desired=2 running=1 pending=1
t+95044ms ecs_running_count_increased desired=2 running=2 pending=0

summary
scaling_activity_seen_ms=65432
ecs_desired_count_changed_ms=67001
ecs_running_count_increased_ms=95044
```

## 测量前检查

确认 service 已注册为 scalable target：

```bash
aws application-autoscaling describe-scalable-targets \
  --service-namespace ecs \
  --scalable-dimension ecs:service:DesiredCount \
  --resource-ids service/<cluster-name>/<service-name>
```

确认有 scaling policy：

```bash
aws application-autoscaling describe-scaling-policies \
  --service-namespace ecs \
  --resource-id service/<cluster-name>/<service-name> \
  --scalable-dimension ecs:service:DesiredCount
```

确认 BenchBox 的负载足够触发扩容。比如 `WORKERS=4` 时：

```bash
curl -X POST http://<benchbox-url>/load \
  -H 'Content-Type: application/json' \
  -d '{"cpu_percent":100,"memory_mb":256}'
```

## 结果解读

如果使用 ECS 服务 CPU/内存指标做 target tracking，延迟通常不会是秒级，因为 ECS 服务指标发布到 CloudWatch 有采样周期，CloudWatch alarm 也需要评估窗口。

如果你想测试“快速扩缩容”的极限，可以考虑：

- 把 scaling policy 的 scale-out cooldown 设置得更短。
- 用 step scaling，并减少 CloudWatch alarm 的 evaluation period。
- 使用更敏感的自定义指标，比如请求队列长度或业务 QPS，而不是只依赖平均 CPU。
- 确保 service 的 `maximum capacity` 大于当前 desired count。
- 确保集群或 capacity provider 有足够资源放置新 task。

## 注意事项

- 建议每组配置至少跑 5 到 10 次，记录 p50、p90、p95。
- 每次测试前先调用 `/reset`，并等待 service 回到稳定状态。
- 如果 scaling activity 出现 `AlreadyAtMaxCapacity`，说明已经达到最大容量，不会继续扩容。
- 如果 `desiredCount` 增加但 `runningCount` 长时间不增加，重点排查任务拉镜像、容量不足、端口、健康检查和 subnet/security group。
