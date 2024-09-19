# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import dataclasses
import pathlib
import unittest

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.analysis import analyze_results
from analyzer.analysis import stats_util


FILES_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute().joinpath("files")
)


class PipelineTest(unittest.TestCase):
    def _result_contains(
        self, result: analysis_results.AnalysisResult, string: str
    ) -> bool:
        for group in result.groups:
            if string in group.sample.metric_path:
                return True
        return False

    def _test_analyze_results_pruning_with_cfg(
        self, cfg: analysis_cfg.AnalysisCfg
    ) -> None:
        results_unpruned = analyze_results.analyze_results(
            [
                FILES_DIR.joinpath("data-complex1.json"),
                FILES_DIR.joinpath("data-complex2.json"),
            ],
            cfg,
        )
        cfg_pruned = dataclasses.replace(
            cfg,
            metric_exclude_regex="2windows",
            metric_include_regex="TabletMode",
            remove_outliers=True,
        )
        results_pruned = analyze_results.analyze_results(
            [
                FILES_DIR.joinpath("data-complex1.json"),
                FILES_DIR.joinpath("data-complex2.json"),
            ],
            cfg_pruned,
        )
        self.assertLess(len(results_pruned), len(results_unpruned))

        for result in results_pruned:
            # If excluded or not included, it should not be in the pruned results.
            assert not self._result_contains(result, "2windows")
            assert self._result_contains(result, "TabletMode")

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
                resamples=99999,
            ),
            bootstrap_params=stats_util.BootstrapParameters(
                deterministic=True,
                resamples=99999,
            ),
            multiple_test_cfg=analysis_cfg.MultipleTestCfg.FWER,
        )
        results = analyze_results.analyze_results(
            [
                FILES_DIR.joinpath("data-complex1.json"),
                FILES_DIR.joinpath("data-complex2.json"),
            ],
            cfg,
        )

        # This additionally checks that identifiers should be unique for each
        # PairwiseResult.
        pairs_by_id = {}
        for result in results:
            for pair in result.pairs:
                assert pair.identifier() not in pairs_by_id
                pairs_by_id[pair.identifier()] = pair
        pair = pairs_by_id[
            "ui.Test.Ash.Overview.AnimationSmoothness.Enter"
            ".ClamshellMode.2windows.average:complex1->complex2"
        ]
        assert pair.before.bootstrap
        assert pair.after.bootstrap
        self.assertAlmostEqual(pair.before.bootstrap.bias_estimate, 0.0022606)
        self.assertAlmostEqual(pair.after.bootstrap.bias_estimate, -0.0028447)

    def _ordered_sample_ids(
        self, results: list[analysis_results.AnalysisResult]
    ) -> list[list[str]]:
        return [
            [group.sample.sample_id for group in result.groups]
            for result in results
        ]

    def test_analyze_results_explicit_experiment_group(self) -> None:
        # Test that explicitly specifying no experiment groups produces no
        # comparisons for samples with the same label.
        cfg = analysis_cfg.AnalysisCfg(
            skip_all_zero_samples=False,
            alpha=1.0,
            experiment_cfg=analysis_cfg.ExperimentCfg(
                experiment_groups_cfgs=[]
            ),
            multiple_test_cfg=analysis_cfg.MultipleTestCfg.NONE,
        )
        results = analyze_results.analyze_results(
            [
                FILES_DIR.joinpath("data-complex1.json"),
            ],
            cfg,
        )
        self.assertEqual(len(results), 0)

        # Test that explicitly specifying experiment groups works. Compare
        # Ash.Overview.AnimationSmoothness.Enter.ClamshellMode with
        # Ash.Overview.AnimationSmoothness.Enter.ClamshellMode.2windows.
        cfg = dataclasses.replace(
            cfg,
            experiment_cfg=analysis_cfg.ExperimentCfg(
                experiment_groups_cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        metric_path_regex_list=[
                            "^.*ClamshellMode\.average$",
                            "^.*ClamshellMode\.2windows\.average$",
                        ],
                    )
                ]
            ),
        )
        results = analyze_results.analyze_results(
            [
                FILES_DIR.joinpath("data-complex1.json"),
            ],
            cfg,
        )
        self.assertEqual(
            [
                [
                    "complex1.ui.Test.Ash.Overview.AnimationSmoothness.Enter.ClamshellMode.average",
                    "complex1.ui.Test.Ash.Overview.AnimationSmoothness.Enter.SingleClamshellMode.average",
                    "complex1.ui.Test.Ash.Overview.AnimationSmoothness.Exit.ClamshellMode.average",
                    "complex1.ui.Test.Ash.Overview.AnimationSmoothness.Exit.SingleClamshellMode.average",
                    "complex1.ui.Test.Ash.Overview.AnimationSmoothness.Enter.ClamshellMode.2windows.average",
                    "complex1.ui.Test.Ash.Overview.AnimationSmoothness.Enter.SingleClamshellMode.2windows.average",
                    "complex1.ui.Test.Ash.Overview.AnimationSmoothness.Exit.ClamshellMode.2windows.average",
                    "complex1.ui.Test.Ash.Overview.AnimationSmoothness.Exit.SingleClamshellMode.2windows.average",
                ]
            ],
            self._ordered_sample_ids(results),
        )

        # Test that explicitly specifying experiment groups with multiple
        # kinds of regex lists works.
        cfg = dataclasses.replace(
            cfg,
            experiment_cfg=analysis_cfg.ExperimentCfg(
                experiment_groups_cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        test_name_regex_list=[
                            "^ui\.Test\.variant$",
                            "^ui\.Test$",
                        ]
                    )
                ]
            ),
        )
        results = analyze_results.analyze_results(
            [
                FILES_DIR.joinpath("data-complex1.json"),
            ],
            cfg,
        )
        # There is one ui.Test.variant sample.
        self.assertEqual(
            [
                [
                    "complex1.ui.Test.variant.Ash.Overview.AnimationSmoothness.Exit.TabletMode.8windows.average",
                    "complex1.ui.Test.Ash.Overview.AnimationSmoothness.Exit.TabletMode.8windows.average",
                ]
            ],
            self._ordered_sample_ids(results),
        )

        # Test that explicitly specifying experiment groups with multiple
        # kinds of regex causes an assertion error.
        cfg = dataclasses.replace(
            cfg,
            experiment_cfg=analysis_cfg.ExperimentCfg(
                experiment_groups_cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        metric_path_regex_list=[
                            "^.*ClamshellMode\.average$",
                        ],
                        test_name_regex_list=[
                            "^ui\.Test\.variant$",
                        ],
                    )
                ]
            ),
        )
        with self.assertRaises(AssertionError):
            analyze_results.analyze_results(
                [
                    FILES_DIR.joinpath("data-complex1.json"),
                ],
                cfg,
            )

    def test_analyze_results_experiment_cfg(self) -> None:
        cfg = analysis_cfg.AnalysisCfg(
            skip_all_zero_samples=False,
            alpha=1.0,
            experiment_cfg=analysis_cfg.ExperimentCfg(per_test_cfgs=[]),
            multiple_test_cfg=analysis_cfg.MultipleTestCfg.NONE,
        )
        results = analyze_results.analyze_results(
            [
                FILES_DIR.joinpath("data-complex1.json"),
                FILES_DIR.joinpath("data-complex2.json"),
            ],
            cfg,
        )
        self.assertEqual(len(results), 43)

        cfg = dataclasses.replace(
            cfg,
            experiment_cfg=analysis_cfg.ExperimentCfg(
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
            [
                FILES_DIR.joinpath("data-complex1.json"),
                FILES_DIR.joinpath("data-complex2.json"),
            ],
            cfg,
        )
        self.assertEqual(len(results), 22)

    def test_analyze_results_empty(self) -> None:
        analyze_results.analyze_results(
            [
                FILES_DIR.joinpath("data-empty.json"),
                FILES_DIR.joinpath("data-empty.json"),
            ],
            analysis_cfg.AnalysisCfg(),
        )
