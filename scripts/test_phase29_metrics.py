"""Failure cases precede implementation: reset clocks, OR gate, empty evidence."""
import unittest

from phase29_metrics import summarize_windows, comparison_gate


def sample(start, elapsed=100, hit=False):
    return {"start_ms": start, "planned_ms": start, "elapsed_ms": elapsed,
            "ok": True, "hit": hit, "concurrent": 1, "stages": {}}


class Phase29MetricsTest(unittest.TestCase):
    def test_reset_clocks_do_not_multiply_throughput(self):
        windows = [[sample(0), sample(900)] for _ in range(6)]
        result = summarize_windows(windows)
        self.assertEqual(result["n"], 12)
        self.assertEqual(result["measurement_elapsed_ms"], 6000)
        self.assertEqual(result["actual_rps"], 2)
        self.assertEqual(result["latency"]["p95_ms"], 100)

    def test_hit_and_miss_keep_full_window_denominator(self):
        result = summarize_windows([[sample(0, 5, True), sample(900)]])
        self.assertEqual(result["hit_only"]["latency"]["p95_ms"], 5)
        self.assertEqual(result["miss_only"]["n"], 1)
        self.assertEqual(result["miss_only"]["actual_rps"], 1)

    def test_candidate_requires_both_bounds(self):
        result = comparison_gate(2200, 2000)
        self.assertFalse(result["candidate_passed"])
        self.assertEqual(result["candidate_limit_ms"], 2040)
        self.assertFalse(result["original_passed"])

    def test_missing_evidence_cannot_pass(self):
        with self.assertRaises(ValueError):
            summarize_windows([[]])
        with self.assertRaises(ValueError):
            comparison_gate(0, 0)


if __name__ == "__main__":
    unittest.main()
