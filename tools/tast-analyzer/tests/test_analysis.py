# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import copy
import dataclasses
import pathlib
import unittest

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.analysis import analyze_results
from analyzer.analysis import metric_sample
from analyzer.analysis import stats_util
from analyzer.backend import test_result
from tests import test_util


FILES_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute().joinpath("files")
)


class AnalysisTest(unittest.TestCase):
    def test_load_samples_from_test_results(self) -> None:
        # Test that a large time-series-like test result has its arithmetic mean
        # taken.
        results = test_result.TestResults(
            results={
                test_result.TestResultKey(
                    run_id="1",
                    test_name="test",
                    metric_name="metric",
                    variant="variant",
                    label="label",
                ): test_result.TestResult(
                    units="s",
                    improvement_direction=test_result.ImprovementDirection.DOWN,
                    value=[1.0] * 100,
                )
            }
        )
        samples = analyze_results._load_samples_from_test_results(results)
        self.assertEqual(len(samples), 1)
        self.assertEqual(samples[0]._value_map, {"1": [1.0]})

    def test_load_samples(self) -> None:
        before_samples = test_util.load_before_samples()

        self.assertEqual(
            before_samples,
            [
                metric_sample.MetricSample(
                    label="before",
                    sample_id="before|ui.OverviewPerf|Test.One.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.One.average",
                    metric_path="ui.OverviewPerf|Test.One.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    _value_map={"before": [0]},
                ),
                metric_sample.MetricSample(
                    label="before",
                    sample_id="before|ui.OverviewPerf|Test.Three.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.Three.average",
                    metric_path="ui.OverviewPerf|Test.Three.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    _value_map={"before": [1, 2, 3]},
                ),
                metric_sample.MetricSample(
                    label="before",
                    sample_id="before|ui.OverviewPerf|Test.Two.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.Two.average",
                    metric_path="ui.OverviewPerf|Test.Two.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    _value_map={"before": [2]},
                ),
            ],
        )

        after_samples = test_util.load_after_samples()
        self.assertEqual(
            after_samples,
            [
                metric_sample.MetricSample(
                    label="after",
                    sample_id="after|ui.OverviewPerf|Test.Four.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.Four.average",
                    metric_path="ui.OverviewPerf|Test.Four.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    _value_map={"after": [2]},
                ),
                metric_sample.MetricSample(
                    label="after",
                    sample_id="after|ui.OverviewPerf|Test.One.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.One.average",
                    metric_path="ui.OverviewPerf|Test.One.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    _value_map={"after": [1]},
                ),
                metric_sample.MetricSample(
                    label="after",
                    sample_id="after|ui.OverviewPerf|Test.Three.average",
                    test_name="ui.OverviewPerf",
                    metric_name="Test.Three.average",
                    metric_path="ui.OverviewPerf|Test.Three.average",
                    units="percent",
                    improvement_direction=metric_sample.ImprovementDirection.UP,
                    _value_map={"after": [0, 1, 2]},
                ),
            ],
        )

    def test_identifier_consistency(self) -> None:
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        for s in samples:
            # Given the tuple (label, test name, metric name), check the
            # definitions:
            # 1. metric path = (test name, metric name)
            # 2. sample id = (label, test name, metric name)
            self.assertEqual(
                s.metric_path, s.test_name + test_result.DELIM + s.metric_name
            )
            self.assertEqual(
                s.sample_id, s.label + test_result.DELIM + s.metric_path
            )

    def test_construct_experiment_groups_list(self) -> None:
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

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
                            "before|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                ],
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                ],
            ],
        )

    def test_explicit_experiment_group_configuration(self) -> None:
        # With explicit experiment group configuration, we should look at
        # only the explicitly set groups if there is only one label.
        before_samples = test_util.load_before_samples()
        before_samples_by_id = test_util.samples_by_id(before_samples)
        cfg = analysis_cfg.AnalysisCfg(
            experiment_cfg=analysis_cfg.ExperimentCfg(
                experiment_groups_cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        metric_path_regex_list=[
                            r"^ui\.OverviewPerf\|Test\.Three\.average$",
                            r"^ui\.OverviewPerf\|Test\.One\.average$",
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
                            "before|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=before_samples_by_id[
                            "before|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                ],
            ],
        )

        cfg = analysis_cfg.AnalysisCfg(
            experiment_cfg=analysis_cfg.ExperimentCfg(
                experiment_groups_cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        test_name_regex_list=[
                            r"^ui\.OverviewPerf$",
                        ]
                    )
                ]
            )
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            before_samples, cfg
        )
        # Should be no groups since there is only one test.
        self.assertEqual(groups_list, [])

        # If there are two labels, look at the explicit experiment groups and
        # the implicit ones between two samples with different labels but the
        # same metric path.
        samples = before_samples + test_util.load_after_samples()
        samples_by_id = test_util.samples_by_id(samples)
        cfg = analysis_cfg.AnalysisCfg(
            experiment_cfg=analysis_cfg.ExperimentCfg(
                experiment_groups_cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        metric_path_regex_list=[
                            r"^ui\.OverviewPerf\|Test\.Three\.average$",
                            r"^ui\.OverviewPerf\|Test\.One\.average$",
                        ]
                    )
                ]
            )
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, cfg
        )
        self.assertEqual(
            groups_list,
            [
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                ],
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                ],
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                ],
            ],
        )

    def test_prune_samples(self) -> None:
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

        # before|ui.OverviewPerf|Test.One.average has only zeros, so we should skip it.
        self.assertEqual(
            analyze_results._prune_all_zero_samples(samples),
            [
                samples_by_id["before|ui.OverviewPerf|Test.Three.average"],
                samples_by_id["before|ui.OverviewPerf|Test.Two.average"],
                samples_by_id["after|ui.OverviewPerf|Test.Four.average"],
                samples_by_id["after|ui.OverviewPerf|Test.One.average"],
                samples_by_id["after|ui.OverviewPerf|Test.Three.average"],
            ],
        )

        # Sample size less than 4 for all metrics, so this should produce nothing.
        self.assertEqual(
            analyze_results._prune_minimum_sample_size(samples, 4), []
        )

    def test_split_better_and_worse_by_mean(self) -> None:
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg()
        )
        self.assertEqual(
            groups_list,
            [
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                ],
                [
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "before|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                    analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.Three.average"
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
                            "before|ui.OverviewPerf|Test.One.average"
                        ]
                    ),
                    after=analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.One.average"
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
                            "before|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                    after=analysis_results.ExperimentGroup(
                        sample=samples_by_id[
                            "after|ui.OverviewPerf|Test.Three.average"
                        ]
                    ),
                    hypothesis_result=stats_util.HypothesisTestResult(
                        statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                        u=7.0,
                        p=0.36868826936178156,
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
            _value_map={},
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

        # Test that using a negative value does not prune results but still
        # adjusts p-values.
        cfg = dataclasses.replace(cfg, alpha=-1.0)
        pruned = analyze_results._prune_non_significant_results(results, cfg)
        self.assertEqual(len(pruned), len(results))
        # Check p-values were adjusted.
        self.assertAlmostEqual(pruned[0].pairs[0].hypothesis_result.p, 0.018)
        self.assertAlmostEqual(pruned[0].pairs[1].hypothesis_result.p, 0.018)
        self.assertAlmostEqual(pruned[0].pairs[2].hypothesis_result.p, 0.018)
        self.assertAlmostEqual(pruned[1].pairs[0].hypothesis_result.p, 0.03)
        self.assertAlmostEqual(pruned[1].pairs[1].hypothesis_result.p, 0.03)
        self.assertAlmostEqual(pruned[1].pairs[2].hypothesis_result.p, 0.03)

    def test_prune_experiment_cfg(self) -> None:
        samples = test_util.load_before_samples()
        samples_by_id = test_util.samples_by_id(samples)

        cfg = analysis_cfg.ExperimentCfg()
        no_change = analyze_results._prune_experiment_cfg(samples, cfg)
        self.assertEqual(no_change, samples)

        cfg = analysis_cfg.ExperimentCfg(
            per_test_cfgs=[
                analysis_cfg.PerTestCfg(
                    test_name_regex="ui\\.OverviewPerf",
                    metric_name_regex_allowlist=[r"Test\.One"],
                )
            ]
        )
        only_one = analyze_results._prune_experiment_cfg(samples, cfg)

        self.assertEqual(
            only_one, [samples_by_id["before|ui.OverviewPerf|Test.One.average"]]
        )

    def test_prune_regex_include(self) -> None:
        before_samples = test_util.load_before_samples()
        before_samples_by_id = test_util.samples_by_id(before_samples)

        self.assertEqual(
            before_samples,
            analyze_results._prune_regex_include(before_samples, "Test.*"),
        )
        self.assertEqual(
            [], analyze_results._prune_regex_include(before_samples, "^Test$")
        )
        self.assertEqual(
            [before_samples_by_id["before|ui.OverviewPerf|Test.Three.average"]],
            analyze_results._prune_regex_include(
                before_samples, r"Test\.Three"
            ),
        )
        self.assertEqual(
            [before_samples_by_id["before|ui.OverviewPerf|Test.Three.average"]],
            analyze_results._prune_regex_include(before_samples, "Test.*ee"),
        )
        self.assertEqual(
            [before_samples_by_id["before|ui.OverviewPerf|Test.Two.average"]],
            analyze_results._prune_regex_include(before_samples, "Test.*o"),
        )

        after_samples = test_util.load_after_samples()
        after_samples_by_id = test_util.samples_by_id(after_samples)
        self.assertEqual(
            [after_samples_by_id["after|ui.OverviewPerf|Test.Three.average"]],
            analyze_results._prune_regex_include(
                after_samples, r"^ui\.OverviewPerf\|Test\.Three\.average$"
            ),
        )

    def test_prune_regex_exclude(self) -> None:
        before_samples = test_util.load_before_samples()
        before_samples_by_id = test_util.samples_by_id(before_samples)

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
                before_samples_by_id["before|ui.OverviewPerf|Test.One.average"],
                before_samples_by_id["before|ui.OverviewPerf|Test.Two.average"],
            ],
            analyze_results._prune_regex_exclude(
                before_samples, r"Test\.Three"
            ),
        )
        self.assertEqual(
            [
                before_samples_by_id["before|ui.OverviewPerf|Test.One.average"],
                before_samples_by_id["before|ui.OverviewPerf|Test.Two.average"],
            ],
            analyze_results._prune_regex_exclude(before_samples, "Test.*ee"),
        )
        self.assertEqual(
            [
                before_samples_by_id["before|ui.OverviewPerf|Test.One.average"],
                before_samples_by_id[
                    "before|ui.OverviewPerf|Test.Three.average"
                ],
            ],
            analyze_results._prune_regex_exclude(before_samples, "Test.*o"),
        )

        after_samples = test_util.load_after_samples()
        after_samples_by_id = test_util.samples_by_id(after_samples)
        self.assertEqual(
            [
                after_samples_by_id["after|ui.OverviewPerf|Test.Four.average"],
                after_samples_by_id["after|ui.OverviewPerf|Test.One.average"],
            ],
            analyze_results._prune_regex_exclude(
                after_samples, r"^ui\.OverviewPerf\|Test\.Three\.average$"
            ),
        )

    def test_prune_outliers(self) -> None:
        samples = [
            metric_sample.MetricSample(
                label="placeholder",
                sample_id="placeholder",
                test_name="test.name",
                metric_name="metric.variant",
                metric_path="test.name.metric.variant",
                units="percent",
                improvement_direction=metric_sample.ImprovementDirection.UP,
                _value_map={},
            )
        ]
        samples_pruned = copy.deepcopy(samples)
        self.assertEqual(
            samples_pruned,
            analyze_results._prune_outliers(samples),
        )

        samples[0]._value_map["test1"] = [1]
        self.assertEqual(
            samples_pruned,
            analyze_results._prune_outliers(samples),
        )

        samples[0]._value_map["test2"] = [2]
        self.assertEqual(
            samples_pruned,
            analyze_results._prune_outliers(samples),
        )

        # Remove highest and lowest.
        samples[0]._value_map["test3"] = [3]
        samples_pruned[0]._value_map["test2"] = [2]
        self.assertEqual(
            samples_pruned,
            analyze_results._prune_outliers(samples),
        )
