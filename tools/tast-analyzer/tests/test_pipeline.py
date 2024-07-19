# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import dataclasses
import pathlib
import unittest

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analyze_results
from analyzer.analysis import stats_util


FILES_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute().joinpath("files")
)


class PipelineTest(unittest.TestCase):
    def _test_analyze_results_pruning_with_cfg(
        self, cfg: analysis_cfg.AnalysisCfg
    ) -> None:
        results_unpruned = analyze_results.analyze_results(
            FILES_DIR.joinpath("data-complex1.json"),
            FILES_DIR.joinpath("data-complex2.json"),
            cfg,
        )
        cfg_pruned = dataclasses.replace(
            cfg,
            metric_exclude_regex="2windows",
            metric_include_regex="TabletMode",
            remove_outliers=True,
        )
        results_pruned = analyze_results.analyze_results(
            FILES_DIR.joinpath("data-complex1.json"),
            FILES_DIR.joinpath("data-complex2.json"),
            cfg_pruned,
        )
        self.assertLess(len(results_pruned), len(results_unpruned))

        for unpruned in results_unpruned:
            # If excluded or not included, it should not be in the pruned results.
            if (
                "2windows" in unpruned.metric_path()
                or "TabletMode" not in unpruned.metric_path()
            ):
                self.assertTrue(
                    all(
                        unpruned.metric_path() != v.metric_path()
                        for v in results_pruned
                    ),
                    f"Expected {unpruned.metric_path()} to not be in the pruned results.",
                )
            else:
                self.assertTrue(
                    any(
                        unpruned.metric_path() == v.metric_path()
                        for v in results_pruned
                    ),
                    f"Expected {unpruned.metric_path()} to be in the pruned results.",
                )

    def test_analyze_results_pruning(self) -> None:
        rank_sum_cfg = analysis_cfg.AnalysisCfg(
            skip_all_zero_samples=False,
            minimum_sample_size=1,
            alpha=1.0,
            hypothesis_test_params=stats_util.HypothesisTestParameters(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM
            ),
            bootstrap_params=stats_util.BootstrapParameters(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM
            ),
            multiple_test_cfg=analysis_cfg.MultipleTestCfg.FWER,
            metric_exclude_regex=None,
            metric_include_regex=None,
            remove_outliers=False,
        )
        self._test_analyze_results_pruning_with_cfg(rank_sum_cfg)

        mean_cfg = dataclasses.replace(
            rank_sum_cfg,
            hypothesis_test_params=stats_util.HypothesisTestParameters(
                statistic_kind=stats_util.TestStatisticKind.MEAN
            ),
            bootstrap_params=stats_util.BootstrapParameters(
                statistic_kind=stats_util.TestStatisticKind.MEAN
            ),
        )
        self._test_analyze_results_pruning_with_cfg(mean_cfg)

    def test_analyze_results_bias_estimate(self) -> None:
        cfg = analysis_cfg.AnalysisCfg(
            skip_all_zero_samples=False,
            alpha=1.0,
            hypothesis_test_params=stats_util.HypothesisTestParameters(
                deterministic=True,
            ),
            bootstrap_params=stats_util.BootstrapParameters(
                deterministic=True,
            ),
            multiple_test_cfg=analysis_cfg.MultipleTestCfg.FWER,
        )
        results = analyze_results.analyze_results(
            FILES_DIR.joinpath("data-complex1.json"),
            FILES_DIR.joinpath("data-complex2.json"),
            cfg,
        )
        results_by_path = {v.metric_path(): v for v in results}
        result = results_by_path[
            "ui.Test.Ash.Overview.AnimationSmoothness.Enter"
            ".ClamshellMode.2windows.average"
        ]
        assert result.before_bootstrap
        assert result.after_bootstrap
        self.assertAlmostEqual(result.before_bootstrap.bias_estimate, 0.0022606)
        self.assertAlmostEqual(result.after_bootstrap.bias_estimate, -0.0028447)

    def test_analyze_results_persistent_cfg(self) -> None:
        cfg = analysis_cfg.AnalysisCfg(
            skip_all_zero_samples=False,
            alpha=1.0,
            persistent_cfg=analysis_cfg.PersistentCfg(per_test_cfgs=[]),
            multiple_test_cfg=analysis_cfg.MultipleTestCfg.NONE,
        )
        results = analyze_results.analyze_results(
            FILES_DIR.joinpath("data-complex1.json"),
            FILES_DIR.joinpath("data-complex2.json"),
            cfg,
        )
        self.assertEqual(len(results), 43)

        cfg = dataclasses.replace(
            cfg,
            persistent_cfg=analysis_cfg.PersistentCfg(
                per_test_cfgs=[
                    analysis_cfg.PerTestCfg(
                        test_name_regex="ui\\.Test",
                        metric_name_regex_allowlist=[
                            "^Ash\\.Overview\\.AnimationSmoothness\\.Enter\\..*$"
                        ],
                    )
                ]
            ),
        )
        results = analyze_results.analyze_results(
            FILES_DIR.joinpath("data-complex1.json"),
            FILES_DIR.joinpath("data-complex2.json"),
            cfg,
        )
        self.assertEqual(len(results), 22)
