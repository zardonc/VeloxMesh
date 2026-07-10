# Scheduler Training Tools

Offline tooling for safe scheduler training samples.

## Commands

Run from this directory:

```bash
uv run scheduler-training export --input samples.jsonl --output samples.csv
uv run scheduler-training train --input samples.jsonl --model build/model.json
uv run scheduler-training evaluate --model build/model.json --input samples.jsonl --metrics build/metrics.json
uv run scheduler-training publish --model build/model.json --metrics build/metrics.json --output-dir artifacts --version scheduler-p70-v1
```

The input file must contain completed scheduler training samples produced by the gateway control-state repository. Rows are allowlisted to safe TaskFeature fields and completion labels only.

Runtime artifacts contain:

- `model.onnx`
- `manifest.json`

Runtime artifacts do not include exported datasets or training logs.
