# 规划与复盘资料

整理日期：2026-10-08（America/Vancouver）。本轮新增资料只保留以下三份，阅读结论先打开主复盘文档。

| 文件 | 用途 |
| --- | --- |
| [REVIEW-20261008.md](REVIEW-20261008.md) | 唯一的本轮复盘正文：阶段结论、修复、排查转折、最终数据、未关闭事项和经验 |
| [REVIEW-EVIDENCE-20261008.json](REVIEW-EVIDENCE-20261008.json) | 可提交的精简依据：场景统计、门槛/区间、源码哈希、后端计数、来源与归档清单 |
| 本README | 导航与资料保留规则，不再复制正文结论 |

已有 [PROJECT](PROJECT.md)、[REQUIREMENTS](REQUIREMENTS.md)、[ROADMAP](ROADMAP.md)、[STATE](STATE.md)、[MILESTONES](MILESTONES.md)、[历史复盘](RETROSPECTIVE.md) 及阶段PLAN/UAT/VERIFICATION等档案保持原位。旧状态应按日期和范围解释，当前复盘不能替代正式发布审批。

## 本地资料与Git边界

实验目录 `debug/merge-*`、`debug/phase29-perf-*`、性能连续日志及原始采样保留本地，由 [.gitignore](../.gitignore) 排除新增提交；既有已跟踪历史资料的状态不变。主复盘和精简证据已经保留阅读与核验所需的核心信息，完整重算仍需要本地原始样本。

五份重复总览/清理资料已合并后移除，原文及旧入口版本保存在 [_local_archive/20261008-consolidation](./_local_archive/20261008-consolidation/) 并逐项校验SHA256。归档不随Git提交；其路径映射和哈希同时保存在精简证据中。如需恢复旧文档，应按记录的original路径恢复，旧相对链接才保持原语义。

本地归档和 `.tmp` 源码/工具保全不是异机备份。后续新增实验保留独立标签、失败窗口、真实配置和源码身份，日常复盘只更新主文档及精简证据，避免再次生成多份内容重叠的总览。
