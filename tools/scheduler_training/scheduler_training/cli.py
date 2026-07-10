from __future__ import annotations

import argparse
from pathlib import Path

from .evaluate import evaluate_file
from .export import read_jsonl, write_csv
from .publish import publish_artifact
from .train import train_file


def main() -> None:
    parser = argparse.ArgumentParser(prog="scheduler-training")
    sub = parser.add_subparsers(dest="cmd", required=True)

    export_cmd = sub.add_parser("export")
    export_cmd.add_argument("--input", required=True)
    export_cmd.add_argument("--output", required=True)

    train_cmd = sub.add_parser("train")
    train_cmd.add_argument("--input", required=True)
    train_cmd.add_argument("--model", required=True)

    eval_cmd = sub.add_parser("evaluate")
    eval_cmd.add_argument("--model", required=True)
    eval_cmd.add_argument("--input", required=True)
    eval_cmd.add_argument("--metrics", required=True)

    publish_cmd = sub.add_parser("publish")
    publish_cmd.add_argument("--model", required=True)
    publish_cmd.add_argument("--metrics", required=True)
    publish_cmd.add_argument("--output-dir", required=True)
    publish_cmd.add_argument("--version", required=True)

    args = parser.parse_args()
    if args.cmd == "export":
        write_csv(read_jsonl(Path(args.input)), Path(args.output))
    elif args.cmd == "train":
        train_file(Path(args.input), Path(args.model))
    elif args.cmd == "evaluate":
        evaluate_file(Path(args.model), Path(args.input), Path(args.metrics))
    elif args.cmd == "publish":
        publish_artifact(Path(args.model), Path(args.metrics), Path(args.output_dir), args.version, {})
