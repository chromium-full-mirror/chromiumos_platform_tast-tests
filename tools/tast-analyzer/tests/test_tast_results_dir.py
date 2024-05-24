# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
from pathlib import Path
import unittest

from analyzer.backend.results import ImprovementDirection
from analyzer.backend.results import TestResult
from analyzer.backend.results import TestResultKey
from analyzer.backend.tast_results_dir import (
    _load_results_from_results_chart_json,
)


FILES_DIR: Path = Path(__file__).parent.absolute().joinpath("files")


class IngestResultsChartTest(unittest.TestCase):
    def test_results_chart(self) -> None:
        path = Path("/20231007-090228/tests/ui.OverviewPerf/results-chart.json")
        results = _load_results_from_results_chart_json(
            path, FILES_DIR.joinpath("results-chart-basic.json").read_text()
        )
        expected = {
            TestResultKey(
                run_id="20231007-090228",
                test_name="ui.OverviewPerf",
                metric_name="Test.One",
                variant="average",
            ): TestResult(
                units="percent",
                improvement_direction=ImprovementDirection.UP,
                value=1,
            ),
            TestResultKey(
                run_id="20231007-090228",
                test_name="ui.OverviewPerf",
                metric_name="Test.Two",
                variant="average",
            ): TestResult(
                units="percent",
                improvement_direction=ImprovementDirection.UP,
                value=2,
            ),
            TestResultKey(
                run_id="20231007-090228",
                test_name="ui.OverviewPerf",
                metric_name="Test.Three",
                variant="average",
            ): TestResult(
                units="percent",
                improvement_direction=ImprovementDirection.UP,
                value=[1, 2, 3],
            ),
        }
        self.assertEqual(results, expected)


if __name__ == "__main__":
    unittest.main()
