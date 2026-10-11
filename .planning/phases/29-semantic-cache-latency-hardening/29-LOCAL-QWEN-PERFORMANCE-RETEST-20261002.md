---
phase: 29
tested_utc_date: 2026-10-02
branch: codex/phase-27-stream-terminal-settlement
source_revision: 3c2b366a0a81ba46653baff261d1b32d56c19d2e
status: local_model_gate_passed_remote_gate_open
merge_allowed: false
---

# Phase 29 本地 Qwen 性能与网络路径诊断

使用真实应用、HTTP 网关、OpenAI-compatible adapter、SQLite、Redis、Qdrant 和本地模型服务，主聊天模型改为 `qwen2.5-0.5b-instruct`，缓存 embedding 仍为 `text-embedding-embedder_collection`。两个模型均由用户本机 `http://127.0.0.1:1234/v1` 提供，隔离测试主机经 SSH reverse loopback 的 `127.0.0.1:11234/v1` 访问。未使用 mock、Gemini、SANS 或 GPT。`LOCAL` provider 只由验收 runner 的运行时参数选择，没有改动生产配置或 `.env.local`。

## 正式低命中对照

沿用 GPT 复测的用例与设置：每轮独立数据库和缓存命名空间，真实 warmup，32 请求、1 请求/s、客户端并发上限 4，次序 off → on → on repeat → off repeat。第 21 个请求重复已完成的第 1 个问题，两轮 on 都真实命中 1/32，命中完整响应分别为 18.274 和 19.538 ms，答案哈希与首次响应一致。读截止仍为实验性 100 ms，其余缓存隔离参数与 [GPT 复测](29-GPT-PERFORMANCE-RETEST-20261002.md)相同。

| 顺序 | 成功/尝试 | 命中 | 观察并发 | P50 ms | P95 ms | P99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| off | 32/32 | 0 | 1 | 147.086 | 341.514 | 360.634 |
| on | 32/32 | 1 | 1 | 144.211 | 231.116 | 423.945 |
| on repeat | 32/32 | 1 | 1 | 162.720 | 299.498 | 408.752 |
| off repeat | 32/32 | 0 | 1 | 138.347 | 398.112 | 426.920 |

两组均全成功，P95 比值分别为 **0.67674** 和 **0.75230**，均低于 1.05。实测请求速率均约 1.03/s，调度延迟 P95 均低于 2 ms。缓存开启轮的 lookup embedding P95 分别为 36.441 和 28.042 ms，向量查询 P95 分别为 2.955 和 2.207 ms；无 lookup 超时。两轮各有一次启动时 `lookup/vector_error`，发生在异步 warmup 写入的向量集合创建完成前；请求 fail-open 后由真实主模型成功回答。这是冷启动时序观察，尚未证明它是独立产品缺陷。

## 并发诊断

为接近 GPT 轮次的 3–4 个同时在途请求，另以 80 请求、目标 50 ms 间隔、客户端并发上限 4 做两次本地 off/on 诊断。首次诊断的 off/on 为 80/80、80/80 成功，on 命中 3 次，P95 为 322.761/368.359 ms，比值 1.14127。修正仅影响观测值的并发计数顺序后复测，off/on 仍分别为 80/80、80/80 成功，on 命中 3 次，实际最大并发均为 4，P95 为 305.998/350.215 ms，比值 1.14450。后一次 on 的 lookup embedding P95 为 71.166 ms，发生一次约 100 ms 的 lookup 超时；向量查询 P95 为 3.234 ms。

两次目标速率都是 20 请求/s，但后一次 off/on 仅达到 15.716/15.036 请求/s，调度延迟 P95 分别为 945.737/1076.078 ms。客户端并发上限持续饱和，因此这不是同速率、无积压的正式门槛对照；1.14450 只能说明**没有公网 provider 路径时，本地模型和 embedding 共用服务的饱和负载也会出现缓存开启后的性能下降**。首次诊断的并发计数可能短暂误报为 5，保留原始日志，以后一次修正后的结果为并发依据。

## 归因与状态

相同网关代码和请求输入在本地模型、低并发场景下两次通过门槛，而 [GPT 远程主模型复测](29-GPT-PERFORMANCE-RETEST-20261002.md)一组 P95 比值为 1.64953、另一组发生 12 秒客户端超时。这支持进一步调查远程 provider 整体路径，包括公网传输、服务端推理及并发排队；**不能只凭不同模型之间的对照证明公网网络传输是根因**。本地测试仍有 SSH/LAN 转发，且 qwen 与 GPT 的生成速度、输出长度和实际并发不同。并发诊断又表明本地资源竞争与缓存读取预算也可能带来延迟。要单独区分公网传输与远程推理/排队，仍需对同一 GPT 请求记录 admission、上游连接/首字节/完整响应耗时及可比的直接上游时序。

证据：[低命中原始日志及统计](measurements/retest-20261002-local-qwen/load-summary.json)、[首次并发诊断](measurements/retest-20261002-local-qwen-burst/load-summary.json)、[修正计数后的并发诊断](measurements/retest-20261002-local-qwen-burst-r2/load-summary.json)。每轮保留逐请求 JSONL 和缓存操作样本，可用 `node scripts/live-ship-analyze.mjs <证据目录>` 重算。测试二进制与 runner 构建、`go vet -tags phase29preflight`、格式检查通过；后一次 Linux 测试二进制 SHA-256 为 `7fd688596562d4b6fe9926198b0e5834ca6770e892901c1344af0b5881b1a584`。三组日志凭据审计通过，runner 每次结束均确认隔离 Redis、Qdrant、PostgreSQL 已停止。原始 GPT 性能失败仍未关闭，`merge_allowed=false`，不合并 `main`。
