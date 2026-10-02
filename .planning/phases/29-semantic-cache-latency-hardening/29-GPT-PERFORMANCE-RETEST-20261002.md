---
phase: 29
tested_utc_date: 2026-10-02
branch: codex/phase-27-stream-terminal-settlement
source_revision: 3c2b366a0a81ba46653baff261d1b32d56c19d2e
status: gaps_found
merge_allowed: false
---

# Phase 29 GPT 性能复测

在上游 provider 并发限制调整后，只使用 `.env.local` 中的 GPT 主模型 `gpt-6-luna` 和用户指定的本地 embedding `text-embedding-embedder_collection`。四轮均通过真实 `App.New()`、HTTP 网关、实际模型、SQLite、Redis 和 Qdrant；没有 mock 模型或服务。Gemini、SANS 均未调用。客户端和应用运行在隔离测试主机，本地 embedding 通过 SSH reverse loopback 到 `127.0.0.1:1234`。

本轮先修正测试负载：第 21 个请求重复第 1 个问题，而不是重复仍在执行的第 20 个请求。两次 cache-on 均实际命中 1/32，完整响应分别为 53.889 和 53.712 ms，答案哈希与首次请求相同。每轮独立数据库和缓存命名空间，先实际 warmup，再以 1 请求/s、客户端并发上限 4 发出 32 个请求。次序为 off、on、on repeat、off repeat。缓存参数仍是隔离测试候选值：100 ms 读截止、4 并发读、2 写 worker、32 队列、2 s 写截止、1 s shutdown grace；未批准为生产值。

| 顺序 | 成功/尝试 | 命中 | 实际请求/s | P50 ms | P95 ms | P99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| off | 32/32 | 0 | 0.979 | 1526.770 | 2543.224 | 2693.391 |
| on | 32/32 | 1 | 0.942 | 1658.634 | 4195.126 | 5444.790 |
| on repeat | 31/32 | 1 | 0.951 | 1775.682 | 3341.617 | 3492.093 |
| off repeat | 32/32 | 0 | 0.863 | 1664.968 | 3339.488 | 6087.065 |

首组全部成功，但 **P95 比值 4195.126 / 2543.224 = 1.64953 > 1.05，门槛失败**。第二组 on 的第 5 个请求在等待 HTTP 响应头时达到 12 秒客户端期限，故 3341.617 / 3339.488 = 1.00064 只是成功响应的描述统计，**不构成通过的性能对照**。原始失败请求保留为 12000.345 ms，不从全部尝试中删除。

本轮将既有缓存操作观测器接入真实负载。首轮 on 的 lookup embedding P50/P95 为 51.005/63.676 ms（32 次，1 次约 100 ms 超时），向量查询 P95 为 3.073 ms（31 次）；on repeat 的 lookup embedding P50/P95 为 50.002/62.815 ms（32 次，0 超时），向量查询 P95 为 2.492 ms（32 次）。两轮均有一次实际缓存命中；异步存储记录成功。on repeat 的失败请求在约 4001.582 ms 开始 embedding，49.249 ms 后完成，随后的向量查询用 1.485 ms，之后仍等待到客户端 12 秒期限。因此这次超时不能归因于本次缓存读取超时；尚无逐请求的 admission、上游和结算分段耗时，不能进一步将等待时间定责给上游 provider 或网关排队。首组 P95 差异也不能从这些缓存操作样本单独解释。

测试用例由 60 秒 Go 测试超时和外层 60 秒进程超时共同限制；格式检查、`go vet -tags phase29preflight ./internal/app`、带 tag 的 Linux 测试二进制编译和 runner 构建均通过。runner 在结束时确认 `veloxmesh-test-redis`、`veloxmesh-test-qdrant`、`veloxmesh-test-postgres` 均为 `false`；凭据审计通过。应用测试二进制 SHA-256 为 `7323211d8462c0d4c1c0309dfac11ed71b73bf18cd3bceb8bd7daeaecb1c6aaa`。

原始逐轮日志、JSONL 及可重算结果保存在 [retest-20261002-gpt](measurements/retest-20261002-gpt/)，汇总见 [load-summary.json](measurements/retest-20261002-gpt/load-summary.json)。统计可用 `node scripts/live-ship-analyze.mjs .planning/phases/29-semantic-cache-latency-hardening/measurements/retest-20261002-gpt` 重算；该脚本会重写同目录解析文件。当前 Phase 29 性能门槛仍未通过，`merge_allowed=false`，不合并 `main`。
