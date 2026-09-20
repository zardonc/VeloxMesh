# ONNX model artifacts

Put the active scheduler model under `deploy/models/current` before starting
the ONNX scheduler.

Required files:

- `model.onnx`
- `manifest.json`

Recommended version layout:

```text
deploy/models/
  scheduler-p70-v1/
    model.onnx
    manifest.json
  current/
    model.onnx
    manifest.json
```

Do not commit real model artifacts unless the project explicitly decides to
version them in Git. The default `.gitignore` keeps model directories local.
