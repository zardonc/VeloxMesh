严重：正常评分被误标为 fallback，可能触发健康 scheduler 自我熔断
影响：单机场景、Scheduler enabled、外部 heuristic/ONNX scorer。
[heuristic/score.go (line 33)](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/scheduler/heuristic/score.go:33) 把 classification.Source 写进 FallbackReason；[heuristic/server.go (line 31)](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/scheduler/heuristic/server.go:31) 又把它放到 proto Reason；[types.go (line 141)](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/scheduler/types.go:141) 会把 Reason 解析回 FallbackReason。随后 [client.go (line 161)](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/scheduler/client.go:161) 只要 FallbackReason != "" 就把 scorer call 记为失败。ONNX 正常路径也写 FallbackReason = "onnx"：[onnx/scorer.go (line 63)](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/scheduler/onnx/scorer.go:63)。
修复方向：拆分 fallback_reason 和 classification_source；正常 heuristic/ONNX 评分必须让 FallbackReason 为空。短期最小修复是在 GRPCScorer 只把明确失败原因计为失败。

严重：scorer/predictor circuit breaker 非线程安全
影响：单机场景默认 SCHEDULER_SCORER_MAX_CONCURRENCY=4，并发 Score 会读写同一个 breaker。
[client.go (line 339)](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/scheduler/client.go:339) 和 [predictor/breaker.go (line 5)](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/scheduler/predictor/breaker.go:5) 的 ring buffer/counters 没有锁，但 [client.go (line 130)](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/scheduler/client.go:130) 和 [python_client.go (line 79)](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/scheduler/predictor/python_client.go:79) 可并发调用。
修复方向：给 breaker 加 sync.Mutex，或改成单一锁保护的状态机；补并发测试，并在有 C 编译器环境下跑 go test -race。


可以。长期方案不要只是“把字段名改一下、加个锁”，而是把 **评分结果语义** 和 **并发状态机** 两个边界正式收口。

**Bug 1：评分结果语义混乱**

根因不是某一行写错，而是 `ScoreResult.FallbackReason` 同时承担了三种含义：

- 正常分类来源：`structured` / `rule` / `fallback`
- 正常 scorer 类型：`onnx`
- 真正降级原因：`timeout` / `breaker_open` / `scorer_busy`

长期方案：把 scheduler result contract 拆成独立字段。

建议模型：

```go
type ScoreResult struct {
    TaskID               string
    Score                float64
    Priority             PriorityClass
    PredictedLatencyMs   int64
    Confidence           float64
    SchedulerVersion     string
    SchedulerType        SchedulerType

    ClassificationSource string // structured | rule | fallback
    FallbackReason       string // empty means scorer result is usable
    AnomalyStatus        string
}
```

proto 也应同步拆分：

```proto
message ScoreResult {
  string task_id = 1;
  double score = 2;
  string priority = 3;
  int64 predicted_latency_ms = 4;
  double confidence = 5;
  string scheduler_version = 6;

  string scheduler_type = 7;
  string classification_source = 8;
  string fallback_reason = 9;
  string anomaly_status = 10;
}
```

长期规则：

- `FallbackReason == ""` 才表示正常 scorer 结果。
- `classification_source="fallback"` 只是分类器冷启动来源，不等于系统降级。
- `SchedulerTypeONNX` / `SchedulerTypeHeuristic` 不应写进 fallback 字段。
- gateway 的 `success` 判断只看 `FallbackReason` 和 RPC error，不看 classification source。
- metrics 里 `classification_source` 和 `scheduler_call_result` 分开打点。

迁移方式：

1. 先在 Go struct 增加 `ClassificationSource`，保留旧 proto `Reason` 兼容。
2. 客户端解析时只把已知错误值映射为 `FallbackReason`，其余映射为 `ClassificationSource`。
3. 新 proto 加字段后，server 写新字段，client 优先读新字段。
4. 最后废弃 `Reason` 的混合语义。

这个方案比到处 special-case `"structured"`、`"rule"`、`"onnx"` 更干净，也避免下一次新增 scorer 又踩同一个坑。

**Bug 2：breaker 并发安全**

长期方案是把 breaker 做成一个小型、线程安全、可观测的组件，不让各客户端复制粘贴状态机。

建议新增一个共享包或 scheduler 内部组件：

```go
type Breaker struct {
    mu       sync.Mutex
    events   []bool
    next     int
    count    int
    failures int
    openedAt time.Time
    cfg      BreakerConfig
}

func (b *Breaker) Allow(now time.Time) bool
func (b *Breaker) Record(now time.Time, success bool) BreakerSnapshot
func (b *Breaker) Snapshot(now time.Time) BreakerSnapshot
```

关键点：

- `Allow`、`Record`、`Snapshot` 全部加锁。
- 所有时间通过 `now time.Time` 注入，测试不用 sleep。
- `Record` 返回 snapshot，调用方顺手更新 metrics。
- gateway `GRPCScorer` 和 predictor `PythonONNXPredictorClient` 共用同一个 breaker 实现。
- `scorer_busy` 不进入 breaker；`timeout`、RPC error、slow success 进入 breaker。
- half-open 同一时间只允许一个探测请求，避免恢复期并发洪水。

half-open 建议加一个 probe slot：

```go
type Breaker struct {
    ...
    probing bool
}
```

语义：

- `open` 且 recovery 未到：拒绝。
- `open` 且 recovery 到：第一个请求进入 `half_open/probing`。
- probing 期间其他请求快速 fallback：`breaker_open` 或 `breaker_probe_in_progress`。
- probe success：reset 到 closed。
- probe failure：重新 open。

测试矩阵：

- 并发 `Record` 不 data race。
- 并发 half-open 只允许一个 probe。
- `scorer_busy` 不打开 breaker。
- slow success 打开 breaker。
- failure window 达阈值打开 breaker。
- success half-open 清空历史窗口。
- Prometheus breaker gauge 随 snapshot 更新。

**发布前最小落地顺序**

1. 先拆 `FallbackReason` / `ClassificationSource`，修正健康 scorer 被误判失败。
2. 把两个 breaker 合并成线程安全实现。
3. 在 `Score` / `Predict` 每次结束后刷新 breaker metrics。
4. 补 race 环境验证；当前机器缺 `gcc`，需要能跑 `go test -race` 的 CI 或本机工具链。

这两个 bug 的共同长期目标：让“正常但低置信度”“正常但冷启动分类”“真正降级失败”三种状态在类型层面分开。现在它们挤在一个字符串里，系统自然会误判。