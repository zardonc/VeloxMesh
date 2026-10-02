---
phase: 29
tested_utc_date: 2026-10-02
status: concurrent_performance_gate_failed
merge_allowed: false
---

# Phase 29 本地并发上限调整后复测

在用户调整本地模型并发上限后，复用此前已验证的真实 `App.New()`、HTTP 网关、OpenAI-compatible adapter、SQLite、Redis、Qdrant 和 SSH 回环转发。主模型为本地 `qwen2.5-0.5b-instruct`，缓存 embedding 为本地 `text-embedding-embedder_collection`；没有 mock 或远程聊天 provider。测试二进制与 [上次本地复测](29-LOCAL-QWEN-PERFORMANCE-RETEST-20261002.md)的修正计数轮相同，SHA-256 为 `7fd688596562d4b6fe9926198b0e5834ca6770e892901c1344af0b5881b1a584`。缓存参数与请求内容未变，四轮均使用独立数据库、真实 warmup 和最多 4 个客户端在途请求。

| 目标速率、每轮样本 | 顺序 | 成功/尝试 | 命中 | 实际请求/s | 调度延迟 P95 ms | 完整响应 P95 ms |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 20/s、80 | off | 80/80 | 0 | 15.392 | 1018.730 | 325.155 |
| 20/s、80 | on | 80/80 | 3 | 14.477 | 1243.187 | 371.177 |
| 20/s、80 | on repeat | 80/80 | 3 | 14.995 | 1147.661 | 327.558 |
| 20/s、80 | off repeat | 80/80 | 0 | 14.972 | 1203.714 | 320.826 |
| 12.5/s、80 | off | 80/80 | 0 | 12.451 | 1.544 | 209.563 |
| 12.5/s、80 | on | 80/80 | 3 | 12.298 | 1.645 | 279.794 |
| 12.5/s、80 | on repeat | 80/80 | 3 | 12.376 | 1.590 | 267.195 |
| 12.5/s、80 | off repeat | 80/80 | 0 | 12.388 | 1.590 | 214.764 |

20/s 目标下两组 P95 比值为 **1.14154** 和 **1.02098**，但持续达到 4 个在途请求且调度积压约 1 秒，实际吞吐约 15/s；这两组不能作为无积压门槛对照。与调整前同参数诊断（off/on 约 15.716/15.036 请求/s，P95 比值 1.14450）相比，未观察到明显的持续吞吐提升，但少量轮次不能确定本地服务的绝对容量。

12.5/s 目标下，四轮均达到预定负载、调度延迟 P95 < 2 ms、曾达到 4 个在途请求，且 320/320 请求全部成功。两组 P95 比值分别为 **1.33513** 和 **1.24413**，均超过 1.05；on 比 off 的 P95 分别高 70.231 和 52.431 ms。on 的 lookup embedding P95 分别为 37.856 和 31.792 ms，向量查询 P95 分别为 2.596 和 2.323 ms，均无 lookup 超时。异步写入与主模型共享本地服务，不能把跨轮差值全部精确归因于读取，但 miss 的同步 embedding/向量查询是已确认的新增关键路径工作。每轮启动时仍记录一次 `lookup/vector_error`，随后正常 fail-open 并完成请求。

此次复测表明，调整本地模型并发上限后，**在没有远程聊天 provider 或公网模型传输的情况下，Phase 29 低命中缓存仍会在稳定并发负载下超过 1.05 性能门槛**。因此网络传输不是这一失败的必要条件；远程 GPT 测试中的模型推理、网络和排队占比仍需单独分段测量，不能由本地结果反推其确切比例。先前本地 1 请求/s 的两组通过结论仍限于低负载，不覆盖本次并发结果。

原始日志、操作样本和可重算汇总：[20/s 目标](measurements/retest-20261002-local-qwen-concurrency-adjusted/load-summary.json)、[12.5/s 目标](measurements/retest-20261002-local-qwen-concurrency-12rps/load-summary.json)。两组均使用 `-test.timeout 60s` 和外层 60 秒进程限制，凭据审计通过；runner 每次结束确认隔离 Redis、Qdrant、PostgreSQL 已停止。性能问题未解决，`merge_allowed=false`，不合并 `main`。
