Diagnostic only: fixed off / exact-miss / semantic-miss / semantic-hit / off order, 100 requests per arm at 125ms, cap four. Off/miss arms share the synthetic FAQ schedule; hit traffic repeats the same seed and is a separate workload. Keep warmup, early formal requests, both off anchors, actual concurrency, stage costs, failures and drift. This is not a six-block release gate and has no new synchronized host/GPU capture. No memo, retries, threshold changes or hardware resizing.

Actual execution: the first exact-labelled arm was overridden to semantic and is
excluded; corrected off/exact/off runs are separate. The existing hit helper is a
C4 burst, not a 125ms scheduler: 100/100 hits at about 162 RPS, retained only as
functional hit/settlement and concurrency-cost evidence. The initial semantic
bracket drifted -14.37%, so its favorable point estimate cannot pass acceptance.
