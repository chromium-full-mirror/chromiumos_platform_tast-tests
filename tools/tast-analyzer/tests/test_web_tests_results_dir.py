# Copyright 2026 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import pathlib
import unittest

from analyzer.backend import test_result
from analyzer.backend import web_tests_results_dir
from analyzer.backend.perfetto_protos.protos.perfetto.trace_summary import (
    file_pb2,
)
from google.protobuf import text_format


FILES_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute().joinpath("files")
)


def load_v2_metrics(path: pathlib.Path) -> file_pb2.TraceSummary:
    summary = file_pb2.TraceSummary()
    text_format.Parse(path.read_text(encoding="utf-8"), summary)
    return summary


class IngestWebTestsv2MetricsPB(unittest.TestCase):
    def test_simple_no_unspecified_direction(self) -> None:
        summary = load_v2_metrics(FILES_DIR / "v2_metrics.simple.textproto")
        results = web_tests_results_dir._load_results_from_trace_summary(
            summary=summary,
            run_id="run_id",
            test_name="test_name",
            label="label",
            unspecified_direction=None,
        )
        # The Simple example has no direction specified, so there are no results
        # by default.
        expected = test_result.TestResults()
        self.assertEqual(results, expected)

    def test_simple_unspecified_direction_up(self) -> None:
        summary = load_v2_metrics(FILES_DIR / "v2_metrics.simple.textproto")
        results = web_tests_results_dir._load_results_from_trace_summary(
            summary=summary,
            run_id="run_id",
            test_name="test_name",
            label="label",
            unspecified_direction=test_result.ImprovementDirection.UP,
        )
        expected = test_result.TestResults(
            results={
                test_result.TestResultKey(
                    run_id="run_id",
                    test_name="test_name",
                    metric_name="simple",
                    variant="summary",
                    label="label",
                ): test_result.TestResult(
                    units="unknown",
                    improvement_direction=test_result.ImprovementDirection.UP,
                    value=83840.5,
                ),
            }
        )
        self.assertEqual(results, expected)

    def test_simple_unspecified_direction_down(self) -> None:
        summary = load_v2_metrics(FILES_DIR / "v2_metrics.simple.textproto")
        results = web_tests_results_dir._load_results_from_trace_summary(
            summary=summary,
            run_id="run_id",
            test_name="test_name",
            label="label",
            unspecified_direction=test_result.ImprovementDirection.DOWN,
        )
        expected = test_result.TestResults(
            results={
                test_result.TestResultKey(
                    run_id="run_id",
                    test_name="test_name",
                    metric_name="simple",
                    variant="summary",
                    label="label",
                ): test_result.TestResult(
                    units="unknown",
                    improvement_direction=test_result.ImprovementDirection.DOWN,
                    value=83840.5,
                ),
            }
        )
        self.assertEqual(results, expected)

    def test_complex(self) -> None:
        summary = load_v2_metrics(FILES_DIR / "v2_metrics.complex.textproto")
        results = web_tests_results_dir._load_results_from_trace_summary(
            summary=summary,
            run_id="run_id",
            test_name="test_name",
            label="label",
            unspecified_direction=test_result.ImprovementDirection.UP,
        )
        expected = test_result.TestResults(
            results={
                test_result.TestResultKey(
                    run_id="run_id",
                    test_name="test_name",
                    metric_name="one_dimension-a",
                    variant="summary",
                    label="label",
                ): test_result.TestResult(
                    units="MEGABYTES",
                    improvement_direction=test_result.ImprovementDirection.DOWN,
                    value=123.0,
                ),
                test_result.TestResultKey(
                    run_id="run_id",
                    test_name="test_name",
                    metric_name="three_dimensions-a-2-3.14",
                    variant="summary",
                    label="label",
                ): test_result.TestResult(
                    units="COUNT",
                    improvement_direction=test_result.ImprovementDirection.UP,
                    value=2.0,
                ),
                test_result.TestResultKey(
                    run_id="run_id",
                    test_name="test_name",
                    metric_name="three_dimensions-x-y-True",
                    variant="summary",
                    label="label",
                ): test_result.TestResult(
                    units="COUNT",
                    improvement_direction=test_result.ImprovementDirection.UP,
                    value=4.0,
                ),
            }
        )
        self.assertEqual(results, expected)

    def test_multispec(self) -> None:
        summary = load_v2_metrics(FILES_DIR / "v2_metrics.multispec.textproto")
        results = web_tests_results_dir._load_results_from_trace_summary(
            summary=summary,
            run_id="run_id",
            test_name="test_name",
            label="label",
            unspecified_direction=None,
        )
        expected = test_result.TestResults(
            results={
                test_result.TestResultKey(
                    run_id="run_id",
                    test_name="test_name",
                    metric_name="min-duration",
                    variant="summary",
                    label="label",
                ): test_result.TestResult(
                    units="TIME_MILLIS",
                    improvement_direction=test_result.ImprovementDirection.DOWN,
                    value=1.0,
                ),
                test_result.TestResultKey(
                    run_id="run_id",
                    test_name="test_name",
                    metric_name="max-duration",
                    variant="summary",
                    label="label",
                ): test_result.TestResult(
                    units="TIME_MILLIS",
                    improvement_direction=test_result.ImprovementDirection.DOWN,
                    value=10.0,
                ),
                test_result.TestResultKey(
                    run_id="run_id",
                    test_name="test_name",
                    metric_name="avg-duration",
                    variant="summary",
                    label="label",
                ): test_result.TestResult(
                    units="TIME_MILLIS",
                    improvement_direction=test_result.ImprovementDirection.DOWN,
                    value=5.5,
                ),
            }
        )
        self.assertEqual(results, expected)


if __name__ == "__main__":
    unittest.main()
