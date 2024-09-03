# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import copy
import pathlib
import unittest

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.analysis import analyze_results
from analyzer.analysis import metric_sample
from analyzer.analysis import stats_util
from analyzer.backend import tast_results_dir


FILES_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute().joinpath("files")
)


class AnalysisTest(unittest.TestCase):
    def _load_before_samples(self) -> list[metric_sample.MetricSample]:
        results = tast_results_dir._load_results_from_results_chart_json(
            path=pathlib.Path(
                "/before/tests/ui.OverviewPerf/results-chart.json"
            ),
            json_str=FILES_DIR.joinpath(
                "results-chart-analysis1.json"
            ).read_text(),
            label="before",
        )
        return analyze_results._load_samples_from_test_results(results)

    def _load_after_samples(self) -> list[metric_sample.MetricSample]:
        results = tast_results_dir._load_results_from_results_chart_json(
            path=pathlib.Path(
                "/after/tests/ui.OverviewPerf/results-chart.json"
            ),
            json_str=FILES_DIR.joinpath(
                "results-chart-analysis2.json"
            ).read_text(),
            label="after",
        )
        return analyze_results._load_samples_from_test_results(results)

    def _samples_by_id(
        self, samples: list[metric_sample.MetricSample]
    ) -> dict[str, metric_sample.MetricSample]:
        samples_by_id = {}
        for s in samples:
            assert s.sample_id not in samples_by_id
            samples_by_id[s.sample_id] = s
        return samples_by_id

    def test_load_samples(self) -> None:
        before_samples = self._load_before_samples()

        self.assertEqual(
            before_samples,
            [
                metric_sample.MetricSample(
                    label="before",
                    sample_id="before.ui.OverviewPerf.Test.One.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.One",
                    metric_path="ui.OverviewPerf.Test.One.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    value_map={"before": 0},
                ),
                metric_sample.MetricSample(
                    label="before",
                    sample_id="before.ui.OverviewPerf.Test.Three.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.Three",
                    metric_path="ui.OverviewPerf.Test.Three.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    # Currently we take the arithmetic mean of lists of values.
                    value_map={"before": 2},
                ),
                metric_sample.MetricSample(
                    label="before",
                    sample_id="before.ui.OverviewPerf.Test.Two.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.Two",
                    metric_path="ui.OverviewPerf.Test.Two.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    value_map={"before": 2},
                ),
            ],
        )

        after_samples = self._load_after_samples()
        self.assertEqual(
            after_samples,
            [
                metric_sample.MetricSample(
                    label="after",
                    sample_id="after.ui.OverviewPerf.Test.Four.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.Four",
                    metric_path="ui.OverviewPerf.Test.Four.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    value_map={"after": 2},
                ),
                metric_sample.MetricSample(
                    label="after",
                    sample_id="after.ui.OverviewPerf.Test.One.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.One",
                    metric_path="ui.OverviewPerf.Test.One.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    value_map={"after": 1},
                ),
                metric_sample.MetricSample(
                    label="after",
                    sample_id="after.ui.OverviewPerf.Test.Three.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.Three",
                    metric_path="ui.OverviewPerf.Test.Three.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    # Currently we take the arithmetic mean of lists of values.
                    value_map={"after": 1},
                ),
            ],
        )

    def test_construct_experiment_groups_list(self) -> None:
        samples = self._load_before_samples() + self._load_after_samples()
        samples_by_id = self._samples_by_id(samples)

        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg()
        )
        # We should only look at the common metric paths.
        self.assertEqual(
            groups_list,
            [
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                ],
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                ],
            ],
        )

    def test_explicit_experiment_group_configuration(self) -> None:
        # With explicit experiment group configuration, we should look at
        # only the explicitly set groups if there is only one label.
        before_samples = self._load_before_samples()
        before_samples_by_id = self._samples_by_id(before_samples)
        cfg = analysis_cfg.AnalysisCfg(
            persistent_cfg=analysis_cfg.PersistentCfg(
                experiment_groups_cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        metric_path_regex_list=[
                            r"^ui\.OverviewPerf\.Test\.Three\.average$",
                            r"^ui\.OverviewPerf\.Test\.One\.average$",
                        ]
                    )
                ]
            )
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            before_samples, cfg
        )
        self.assertEqual(
            groups_list,
            [
                [
                    analysis_results.ExperimentGroup(
                        sample=before_samples_by_id[
                            "before.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=before_samples_by_id[
                            "before.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                ],
            ],
        )

        # If there are two labels, look at the explicit experiment groups and
        # the implicit ones between two samples with different labels but the
        # same metric path.
        samples = before_samples + self._load_after_samples()
        samples_by_id = self._samples_by_id(samples)
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, cfg
        )
        self.assertEqual(
            groups_list,
            [
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                ],
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                ],
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                ],
            ],
        )

    def test_prune_samples(self) -> None:
        samples = self._load_before_samples() + self._load_after_samples()
        samples_by_id = self._samples_by_id(samples)

        # before.ui.OverviewPerf.Test.One.average has only zeros, so we should skip it.
        self.assertEqual(
            analyze_results._prune_all_zero_samples(samples),
            [
                samples_by_id["before.ui.OverviewPerf.Test.Three.average"],
                samples_by_id["before.ui.OverviewPerf.Test.Two.average"],
                samples_by_id["after.ui.OverviewPerf.Test.Four.average"],
                samples_by_id["after.ui.OverviewPerf.Test.One.average"],
                samples_by_id["after.ui.OverviewPerf.Test.Three.average"],
            ],
        )

        # Sample size is one for all metrics, so this should produce nothing.
        self.assertEqual(
            analyze_results._prune_minimum_sample_size(samples, 2), []
        )

    def test_split_better_and_worse_by_mean(self) -> None:
        samples = self._load_before_samples() + self._load_after_samples()
        samples_by_id = self._samples_by_id(samples)

        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg()
        )
        self.assertEqual(
            groups_list,
            [
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                ],
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                ],
            ],
        )

        results = analysis_results.generate_analysis_results(
            groups_list=groups_list,
            hypothesis_params=stats_util.HypothesisTestParameters(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM
            ),
            bootstrap_params=stats_util.BootstrapParameters(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM
            ),
        )
        better_result = analysis_results.AnalysisResult(
            groups=groups_list[0],
            pairs=[
                analysis_results.PairwiseResult(
                    before=analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                    after=analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.One.average"
                        ]
                    ),
                    hypothesis_result=stats_util.HypothesisTestResult(
                        statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                        u=0.0,
                        p=1.0,
                    ),
                )
            ],
        )
        worse_result = analysis_results.AnalysisResult(
            groups=groups_list[1],
            pairs=[
                analysis_results.PairwiseResult(
                    before=analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                    after=analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after.ui.OverviewPerf.Test.Three.average"
                        ]
                    ),
                    hypothesis_result=stats_util.HypothesisTestResult(
                        statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                        u=1.0,
                        p=1.0,
                    ),
                )
            ],
        )
        self.assertEqual(
            results,
            [
                better_result,
                worse_result,
            ],
        )

        all_pairs = [p for result in results for p in result.pairs]
        (
            better_pairs,
            worse_pairs,
        ) = analysis_results.split_better_and_worse_by_mean(all_pairs)
        self.assertEqual(better_pairs, better_result.pairs)
        self.assertEqual(worse_pairs, worse_result.pairs)

    def _make_analysis_result(
        self, u: float, p: float
    ) -> analysis_results.AnalysisResult:
        placeholder = metric_sample.MetricSample(
            label="placeholder",
            sample_id="placeholder",
            test_name="placeholder",
            metric_name="placeholder",
            metric_path="placeholder",
            units="placeholder",
            improvement_direction=metric_sample.ImprovementDirection.UP,
            value_map={},
        )
        group = analysis_results.ExperimentGroup(sample=placeholder)
        pair = analysis_results.PairwiseResult(
            before=group,
            after=group,
            hypothesis_result=stats_util.HypothesisTestResult(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                u=u,
                p=p,
            ),
        )
        return analysis_results.AnalysisResult(
            groups=[group, group, group],
            pairs=[pair, pair, pair],
        )

    def test_prune_non_significant_results(self) -> None:
        cfg = analysis_cfg.AnalysisCfg(
            alpha=0.03,
            multiple_test_cfg=analysis_cfg.MultipleTestCfg.FWER,
            hypothesis_test_params=stats_util.HypothesisTestParameters(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM
            ),
        )
        results = [
            self._make_analysis_result(u=0.0, p=0.001),
            self._make_analysis_result(u=0.0, p=0.002),
            self._make_analysis_result(u=0.0, p=0.003),
            self._make_analysis_result(u=0.0, p=0.004),
            self._make_analysis_result(u=0.0, p=0.005),
            self._make_analysis_result(u=0.0, p=0.006),
        ]
        pruned = analyze_results._prune_non_significant_results(results, cfg)
        self.assertEqual(len(pruned), 2)
        self.assertEqual(len(pruned[0].pairs), 3)
        self.assertEqual(len(pruned[1].pairs), 3)
        # Check p-values were adjusted.
        self.assertAlmostEqual(pruned[0].pairs[0].hypothesis_result.p, 0.018)
        self.assertAlmostEqual(pruned[0].pairs[1].hypothesis_result.p, 0.018)
        self.assertAlmostEqual(pruned[0].pairs[2].hypothesis_result.p, 0.018)
        self.assertAlmostEqual(pruned[1].pairs[0].hypothesis_result.p, 0.03)
        self.assertAlmostEqual(pruned[1].pairs[1].hypothesis_result.p, 0.03)
        self.assertAlmostEqual(pruned[1].pairs[2].hypothesis_result.p, 0.03)

    def test_prune_persistent_cfg(self) -> None:
        samples = self._load_before_samples()
        samples_by_id = self._samples_by_id(samples)

        cfg = analysis_cfg.PersistentCfg()
        no_change = analyze_results._prune_persistent_cfg(samples, cfg)
        self.assertEqual(no_change, samples)

        cfg = analysis_cfg.PersistentCfg(
            per_test_cfgs=[
                analysis_cfg.PerTestCfg(
                    test_name_regex="ui\\.OverviewPerf",
                    metric_name_regex_allowlist=[r"Test\.One"],
                )
            ]
        )
        only_one = analyze_results._prune_persistent_cfg(samples, cfg)

        self.assertEqual(
            only_one, [samples_by_id["before.ui.OverviewPerf.Test.One.average"]]
        )

    def test_prune_regex_include(self) -> None:
        before_samples = self._load_before_samples()
        before_samples_by_id = self._samples_by_id(before_samples)

        self.assertEqual(
            before_samples,
            analyze_results._prune_regex_include(before_samples, "Test.*"),
        )
        self.assertEqual(
            [], analyze_results._prune_regex_include(before_samples, "^Test$")
        )
        self.assertEqual(
            [before_samples_by_id["before.ui.OverviewPerf.Test.Three.average"]],
            analyze_results._prune_regex_include(
                before_samples, r"Test\.Three"
            ),
        )
        self.assertEqual(
            [before_samples_by_id["before.ui.OverviewPerf.Test.Three.average"]],
            analyze_results._prune_regex_include(before_samples, "Test.*ee"),
        )
        self.assertEqual(
            [before_samples_by_id["before.ui.OverviewPerf.Test.Two.average"]],
            analyze_results._prune_regex_include(before_samples, "Test.*o"),
        )

        after_samples = self._load_after_samples()
        after_samples_by_id = self._samples_by_id(after_samples)
        self.assertEqual(
            [after_samples_by_id["after.ui.OverviewPerf.Test.Three.average"]],
            analyze_results._prune_regex_include(
                after_samples, r"^ui\.OverviewPerf\.Test\.Three\.average$"
            ),
        )

    def test_prune_regex_exclude(self) -> None:
        before_samples = self._load_before_samples()
        before_samples_by_id = self._samples_by_id(before_samples)

        self.assertEqual(
            [],
            analyze_results._prune_regex_exclude(before_samples, "Test.*"),
        )
        self.assertEqual(
            before_samples,
            analyze_results._prune_regex_exclude(before_samples, "^Test$"),
        )
        self.assertEqual(
            [
                before_samples_by_id["before.ui.OverviewPerf.Test.One.average"],
                before_samples_by_id["before.ui.OverviewPerf.Test.Two.average"],
            ],
            analyze_results._prune_regex_exclude(
                before_samples, r"Test\.Three"
            ),
        )
        self.assertEqual(
            [
                before_samples_by_id["before.ui.OverviewPerf.Test.One.average"],
                before_samples_by_id["before.ui.OverviewPerf.Test.Two.average"],
            ],
            analyze_results._prune_regex_exclude(before_samples, "Test.*ee"),
        )
        self.assertEqual(
            [
                before_samples_by_id["before.ui.OverviewPerf.Test.One.average"],
                before_samples_by_id[
                    "before.ui.OverviewPerf.Test.Three.average"
                ],
            ],
            analyze_results._prune_regex_exclude(before_samples, "Test.*o"),
        )

        after_samples = self._load_after_samples()
        after_samples_by_id = self._samples_by_id(after_samples)
        self.assertEqual(
            [
                after_samples_by_id["after.ui.OverviewPerf.Test.Four.average"],
                after_samples_by_id["after.ui.OverviewPerf.Test.One.average"],
            ],
            analyze_results._prune_regex_exclude(
                after_samples, r"^ui\.OverviewPerf\.Test\.Three\.average$"
            ),
        )

    def test_prune_outliers(self) -> None:
        samples = [
            metric_sample.MetricSample(
                label="placeholder",
                sample_id="placeholder",
                test_name="ui.OverviewPerf",
                metric_name="metric",
                metric_path="test.name.metric.path",
                units="percent",
                improvement_direction=metric_sample.ImprovementDirection.UP,
                value_map={},
            )
        ]
        samples_pruned = copy.deepcopy(samples)
        self.assertEqual(
            samples_pruned,
            analyze_results._prune_outliers(samples),
        )

        samples[0].value_map["test1"] = 1
        self.assertEqual(
            samples_pruned,
            analyze_results._prune_outliers(samples),
        )

        samples[0].value_map["test2"] = 2
        self.assertEqual(
            samples_pruned,
            analyze_results._prune_outliers(samples),
        )

        # Remove highest and lowest.
        samples[0].value_map["test3"] = 3
        samples_pruned[0].value_map["test2"] = 2
        self.assertEqual(
            samples_pruned,
            analyze_results._prune_outliers(samples),
        )
