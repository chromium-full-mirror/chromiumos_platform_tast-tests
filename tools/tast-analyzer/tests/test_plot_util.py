# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import unittest

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.frontend import plot_util
from tests import test_util


class PlotUtilTest(unittest.TestCase):
    def _get_groups_with_same_test_metric(
        self,
    ) -> list[analysis_results.ExperimentGroup]:
        """Returns a list of groups from the same test with the same metric."""
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg()
        )
        return groups_list[0]

    def _get_groups_with_different_test(
        self, test_name1: str, test_name2: str
    ) -> list[analysis_results.ExperimentGroup]:
        """Returns a list of groups from different tests with the same metric.

        Args:
            test_name1: A test name.
            test2_name: A different test name.

        Returns:
            A list of groups from different tests with the same metric.
        """
        samples = test_util.load_before_samples(
            test_name1
        ) + test_util.load_after_samples(test_name2)
        experiment_cfg = analysis_cfg.ExperimentCfg(
            experiment_groups_cfgs=[
                analysis_cfg.ExperimentGroupsCfg(
                    test_name_regex_list=[test_name1, test_name2]
                )
            ]
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg(experiment_cfg=experiment_cfg)
        )
        return groups_list[0]

    def _get_groups_with_different_metric(
        self,
    ) -> list[analysis_results.ExperimentGroup]:
        """Returns a list of groups from the same test with different metrics."""
        metric_name1 = "Test.Two.average"
        metric_name2 = "Test.Four.average"
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        experiment_groups_cfgs = [
            analysis_cfg.ExperimentGroupsCfg(
                metric_path_regex_list=[
                    f".*{metric_name1}",
                    f".*{metric_name2}",
                ]
            )
        ]
        groups_list = (
            analysis_results._construct_explicit_experiment_groups_list(
                samples, experiment_groups_cfgs
            )
        )
        return groups_list[0]

    def test_get_groups_name_for_plot(self) -> None:
        metric_path = "Test.One.average"
        groups = self._get_groups_with_same_test_metric()
        result = plot_util.get_groups_name_for_plot(groups)
        self.assertEqual(
            result, f"(before|after)|ui.OverviewPerf|{metric_path}"
        )

        before_test_name = "ui.OverviewPerfBefore"
        after_test_name = "ui.OverviewPerfAfter"
        groups = self._get_groups_with_different_test(
            before_test_name, after_test_name
        )
        result = plot_util.get_groups_name_for_plot(groups)
        self.assertEqual(
            result,
            f"before|{before_test_name}|{metric_path}, "
            f"after|{after_test_name}|{metric_path}",
        )

        groups = self._get_groups_with_different_metric()
        result = plot_util.get_groups_name_for_plot(groups)
        self.assertEqual(
            result,
            "before|ui.OverviewPerf|Test.Two.average, "
            "after|ui.OverviewPerf|Test.Four.average",
        )
