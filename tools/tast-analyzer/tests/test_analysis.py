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

    def test_compute_metric_paths_for_comparison(self) -> None:
        before_samples = self._load_before_samples()
        after_samples = self._load_after_samples()

        metric_paths = analysis_results.compute_metric_paths_for_comparison(
            before_samples, after_samples
        )
        # We should only look at the common metric paths.
        self.assertEqual(
            metric_paths,
            [
                "ui.OverviewPerf.Test.One.average",
                "ui.OverviewPerf.Test.Three.average",
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
        before_samples = self._load_before_samples()
        before_samples_by_id = self._samples_by_id(before_samples)
        after_samples = self._load_after_samples()
        after_samples_by_id = self._samples_by_id(after_samples)

        metric_paths = analysis_results.compute_metric_paths_for_comparison(
            before_samples, after_samples
        )
        self.assertEqual(
            metric_paths,
            [
                "ui.OverviewPerf.Test.One.average",
                "ui.OverviewPerf.Test.Three.average",
            ],
        )

        results = analysis_results.generate_analysis_results(
            before_samples=before_samples,
            after_samples=after_samples,
            metric_paths=metric_paths,
            hypothesis_params=stats_util.HypothesisTestParameters(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM
            ),
            bootstrap_params=stats_util.BootstrapParameters(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM
            ),
        )
        better_result = analysis_results.AnalysisResult(
            before_sample=before_samples_by_id["before." + metric_paths[0]],
            after_sample=after_samples_by_id["after." + metric_paths[0]],
            hypothesis_result=stats_util.HypothesisTestResult(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                u=0.0,
                p=1.0,
            ),
            before_bootstrap=None,
            after_bootstrap=None,
        )
        worse_result = analysis_results.AnalysisResult(
            before_sample=before_samples_by_id["before." + metric_paths[1]],
            after_sample=after_samples_by_id["after." + metric_paths[1]],
            hypothesis_result=stats_util.HypothesisTestResult(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                u=1.0,
                p=1.0,
            ),
            before_bootstrap=None,
            after_bootstrap=None,
        )
        self.assertEqual(
            results,
            [
                better_result,
                worse_result,
            ],
        )

        better, worse = analysis_results.split_better_and_worse_by_mean(results)
        self.assertEqual(better, [better_result])
        self.assertEqual(worse, [worse_result])

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
        return analysis_results.AnalysisResult(
            before_sample=placeholder,
            after_sample=placeholder,
            hypothesis_result=stats_util.HypothesisTestResult(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM, u=u, p=p
            ),
            before_bootstrap=None,
            after_bootstrap=None,
        )

    def test_prune_non_significant_results(self) -> None:
        cfg = analysis_cfg.AnalysisCfg(
            alpha=0.01,
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
        # Check p-values were adjusted.
        self.assertEqual(pruned[0].hypothesis_result.p, 0.006)
        self.assertEqual(pruned[1].hypothesis_result.p, 0.01)

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
