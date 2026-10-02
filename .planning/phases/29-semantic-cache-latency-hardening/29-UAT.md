---
status: complete
phase: 29-semantic-cache-latency-hardening
source: [29-01-SUMMARY.md, 29-02-SUMMARY.md, 29-03-PROGRESS.md]
started: 2026-10-01T20:34:21-07:00
updated: 2026-10-01T20:48:00-07:00
acceptance_scope: local-model-functional
functional_status: passed
production_release_approved: false
---

# Phase 29 功能验收

## Current Test

[testing complete]

用户本次要求：“由于在线embedding模型额度限制，无法完成压测，暂时以local模型完成验证即可，其余非系统功能性异常仅记录报告中。”本轮由代理执行测试，不以对话中的确认代替实测。原在线双模型、持续容量及性能发布门槛保留为延期项，不记作通过，也不阻塞本次功能验收。

## Tests

| # | 验收项目与预期 | 结果 | 本次证据 |
| --- | --- | --- | --- |
| 1 | 默认关闭、未授权请求不使用缓存 | pass | `offline.jsonl`：配置、可信 profile、开发密钥/工具绕过测试；`cache-http.jsonl` |
| 2 | 模型配置可替换，维度校验有效 | pass | 配置/adapter 测试；本地模型真实探测为 768 维 |
| 3 | API key、知识版本、目标模型、embedding provider 隔离，TTL 到期不复用 | pass | `TestSemanticCacheLabeledFAQAndIsolation`、scope/provider 测试 |
| 4 | 缓存命中保持零 Usage，前台响应不等待慢写入 | pass | gateway/cache 回归及 `TestSemanticCache_CacheHeaders` |
| 5 | 读取总截止及并发上限有效，包括不配合取消的依赖 | pass | `TestSemanticCacheReadDeadlineAndSaturation`、`TestSemanticCacheCombinedReadWriteCapacity` |
| 6 | 错误/非法 embedding、向量、仓库数据不导致缓存来源的请求失败，不进行故障后的全表回退 | pass | boundary/vector/repository/malformed-choice 测试 |
| 7 | 异步写入快照不可变，队列和关闭有限，关闭后拒绝新写入 | pass | queue/shutdown/snapshot 测试及本地真实队列测试 |
| 8 | 本地真实 embedding 经正常 App、认证 HTTP 网关及 Qdrant 完成 miss、落库、语义改写 hit | pass | `TestPhase29LocalGatewayAcceptance.log`：改写 1/1 命中、答案哈希相同、59.096 ms |
| 9 | 语义相近但答案不同的退款/取消问题不误命中 | pass | 同一真实流程：2/2 miss；仅代表此已标注小语料 |
| 10 | 相同 key/model/database 的 faq-v1 → faq-v2 立即隔离旧答案 | pass | 同一真实流程：新版本首次请求 miss；不代表生产原子发布程序已验证 |
| 11 | 真实 adapter 探测后注入 embedding HTTP 503，普通主响应继续返回并记录原因 | pass | `TestPhase29LocalEmbeddingFault.log`：HTTP 200、hit=false、lookup/store embedding_error |
| 12 | 128 个候选的队列满行为不等待，关闭有界 | pass | `TestPhase29LocalQueueBurst.log`：34 accepted、94 drop-newest、enqueue 0.040 ms、close 588.635 ms、关闭后拒绝 |
| 13 | 在线第二款 embedding 的完整生命周期 | skipped | 用户授权本轮仅以本地模型验收；原两模型发布证据延期 |
| 14 | 持续容量、稳定并发及低命中 P95 ≤ 1.05 | skipped | 按用户要求不重跑压测；既有 P95 未达标记录保留，不能记作成功 |
| 15 | 生产参数批准、发布负责人及原子发布操作 | skipped | 本轮仅隔离验收；生产启用未授权 |

## Summary

total: 15
passed: 12
issues: 0
pending: 0
skipped: 3
blocked: 0

结论：本次调整范围内的功能验收通过，没有发现系统功能性失败。完整测试统计、复现方式、环境异常及原发布门槛见 `29-VERIFICATION.md`。这里的 `complete` 仅表示本轮 UAT 已结束，不表示原全部发布门槛通过。

## Gaps

[]

当前无新发现的功能缺陷需要生成修复计划。性能和在线模型延期项由验收报告单独记录，不伪装为已解决缺陷。
