# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import pathlib
import unittest

from analyzer.analysis import analyze_results
from analyzer.analysis import metric_sample


FILES_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute().joinpath("files")
)


class MigrationTest(unittest.TestCase):
    def test_results_v1_migration(self) -> None:
        # Test that the label is set to the filename.
        samples = analyze_results.load_samples_from_test_results(
            analyze_results.load_test_results(
                [FILES_DIR.joinpath("data-migration-results-v1.json")]
            )
        )
        self.assertEqual(
            samples,
            [
                metric_sample.MetricSample(
                    label="data-migration-results-v1.json",
                    sample_id="data-migration-results-v1.json|ui.Test|Migration.Results.V1.average",
                    test_name="ui.Test",
                    metric_name="Migration.Results.V1.average",
                    metric_path="ui.Test|Migration.Results.V1.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    _value_map={"1": [1.0]},
                )
            ],
        )
