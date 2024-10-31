# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import pathlib
import unittest

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.analysis import stats_util
from analyzer.backend import test_result
from analyzer.frontend import output
from analyzer.frontend import plot
from analyzer.frontend import plot_util
from analyzer.frontend.report import html_report
from matplotlib import figure
from tests import test_util


TEMPLATE_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.parent.absolute()
    / "configs"
    / "report"
    / "templates"
)
HTML_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute() / "files" / "report" / "html"
)


class HtmlReportTest(unittest.TestCase):
    def test_test_names(self) -> None:
        before_test_name = "ui.OverviewPerfBefore"
        after_test_name = "ui.OverviewPerfAfter"

        samples = test_util.load_before_samples(
            before_test_name
        ) + test_util.load_after_samples(after_test_name)
        samples_by_id = test_util.samples_by_id(samples)

        experiment_cfg = analysis_cfg.ExperimentCfg(
            experiment_groups_cfgs=[
                analysis_cfg.ExperimentGroupsCfg(
                    metric_path_regex_list=[".*Test.One"]
                )
            ]
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg(experiment_cfg=experiment_cfg)
        )
        results = [
            output.AnalysisResultForOutput(
                groups=groups_list[0],
                pairs=[
                    output.PairwiseResultForOutput(
                        result=analysis_results.PairwiseResult(
                            before=analysis_results.ExperimentGroup(
                                sample=samples_by_id[
                                    f"before|{before_test_name}|Test.One.average"
                                ]
                            ),
                            after=analysis_results.ExperimentGroup(
                                sample=samples_by_id[
                                    f"after|{after_test_name}|Test.One.average"
                                ]
                            ),
                            hypothesis_result=stats_util.HypothesisTestResult(
                                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                                u=0.0,
                                p=1.0,
                            ),
                        ),
                        plots=[],
                    ),
                ],
                groups_plots=[],
            )
        ]

        report = html_report.HtmlReport(
            raw_test_results=[],
            results=results,
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        self.assertEqual(
            report._test_names(), [after_test_name, before_test_name]
        )

    def test_labels(self) -> None:
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg()
        )
        results = [
            output.AnalysisResultForOutput(
                groups=groups_list[0],
                pairs=[
                    output.PairwiseResultForOutput(
                        result=analysis_results.PairwiseResult(
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
                        ),
                        plots=[],
                    ),
                ],
                groups_plots=[],
            )
        ]
        report = html_report.HtmlReport(
            raw_test_results=[],
            results=results,
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        self.assertEqual(report._labels(), ["after", "before"])

    def test_metric_paths(self) -> None:
        before_metric_name = "Test.One.average"
        after_metric_name = "Test.Three.average"
        before_metric_path = f"ui.OverviewPerf|{before_metric_name}"
        after_metric_path = f"ui.OverviewPerf|{after_metric_name}"
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

        experiment_cfg = analysis_cfg.ExperimentCfg(
            per_test_cfgs=[
                analysis_cfg.PerTestCfg(
                    test_name_regex="ui\\.OverviewPerf",
                    metric_name_regex_allowlist=[
                        rf"{before_metric_name}|{after_metric_name}"
                    ],
                )
            ]
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg(experiment_cfg=experiment_cfg)
        )
        results = [
            output.AnalysisResultForOutput(
                groups=groups_list[0],
                pairs=[
                    output.PairwiseResultForOutput(
                        result=analysis_results.PairwiseResult(
                            before=analysis_results.ExperimentGroup(
                                sample=samples_by_id[
                                    f"before|{before_metric_path}"
                                ]
                            ),
                            after=analysis_results.ExperimentGroup(
                                sample=samples_by_id[
                                    f"after|{after_metric_path}"
                                ]
                            ),
                            hypothesis_result=stats_util.HypothesisTestResult(
                                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                                u=0.0,
                                p=1.0,
                            ),
                        ),
                        plots=[],
                    ),
                ],
                groups_plots=[],
            )
        ]
        report = html_report.HtmlReport(
            raw_test_results=[],
            results=results,
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        self.assertEqual(
            report._metric_paths(), [before_metric_path, after_metric_path]
        )

    def test_set_title(self) -> None:
        before_test_name = "ui.OverviewPerfBefore"
        after_test_name = "ui.OverviewPerfAfter"

        samples = test_util.load_before_samples(
            before_test_name
        ) + test_util.load_after_samples(after_test_name)
        samples_by_id = test_util.samples_by_id(samples)

        experiment_cfg = analysis_cfg.ExperimentCfg(
            experiment_groups_cfgs=[
                analysis_cfg.ExperimentGroupsCfg(
                    metric_path_regex_list=[r".*Test\.One"]
                )
            ]
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg(experiment_cfg=experiment_cfg)
        )
        results = [
            output.AnalysisResultForOutput(
                groups=groups_list[0],
                pairs=[
                    output.PairwiseResultForOutput(
                        result=analysis_results.PairwiseResult(
                            before=analysis_results.ExperimentGroup(
                                sample=samples_by_id[
                                    f"before|{before_test_name}|Test.One.average"
                                ]
                            ),
                            after=analysis_results.ExperimentGroup(
                                sample=samples_by_id[
                                    f"after|{after_test_name}|Test.One.average"
                                ]
                            ),
                            hypothesis_result=stats_util.HypothesisTestResult(
                                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                                u=0.0,
                                p=1.0,
                            ),
                        ),
                        plots=[],
                    ),
                ],
                groups_plots=[],
            )
        ]
        report = html_report.HtmlReport(
            raw_test_results=[],
            results=results,
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        report._set_title()

        expected_html = test_util.load_html(HTML_DIR / "set_title.html")
        test_util.assert_elements_equal_except_image_data(
            self, report.html.html, expected_html
        )

    def test_create_sample_size_table(self) -> None:
        metric_path = "ui.OverviewPerf|Test.One.average"

        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg()
        )
        results = [
            output.AnalysisResultForOutput(
                groups=groups_list[0],
                pairs=[
                    output.PairwiseResultForOutput(
                        result=analysis_results.PairwiseResult(
                            before=analysis_results.ExperimentGroup(
                                sample=samples_by_id[f"before|{metric_path}"]
                            ),
                            after=analysis_results.ExperimentGroup(
                                sample=samples_by_id[f"after|{metric_path}"]
                            ),
                            hypothesis_result=stats_util.HypothesisTestResult(
                                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                                u=0.0,
                                p=1.0,
                            ),
                        ),
                        plots=[],
                    ),
                ],
                groups_plots=[],
            )
        ]
        report = html_report.HtmlReport(
            raw_test_results=[],
            results=results,
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        table = report._create_sample_size_table()

        expected_table = test_util.load_html(
            HTML_DIR / "sample_size_table.html"
        )
        test_util.assert_elements_equal_except_image_data(
            self, table, expected_table
        )

    def test_append_summary_empty_results(self) -> None:
        report = html_report.HtmlReport(
            raw_test_results=[],
            results=[],
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        report._append_summary()

        expected_html = test_util.load_html(
            HTML_DIR / "append_summary_empty_results.html"
        )
        test_util.assert_elements_equal_except_image_data(
            self, report.html.html, expected_html
        )

    def test_append_summary_with_results(self) -> None:
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg()
        )
        pair = analysis_results.PairwiseResult(
            before=analysis_results.ExperimentGroup(
                sample=samples_by_id["before|ui.OverviewPerf|Test.One.average"]
            ),
            after=analysis_results.ExperimentGroup(
                sample=samples_by_id["after|ui.OverviewPerf|Test.One.average"]
            ),
            hypothesis_result=stats_util.HypothesisTestResult(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                u=0.0,
                p=1.0,
            ),
        )
        results = [
            output.AnalysisResultForOutput(
                groups=groups_list[0],
                pairs=[output.PairwiseResultForOutput(result=pair, plots=[])],
                groups_plots=[],
            ),
        ]
        report = html_report.HtmlReport(
            raw_test_results=[],
            results=results,
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        report._append_summary()

        expected_html = test_util.load_html(
            HTML_DIR / "append_summary_with_results.html"
        )
        test_util.assert_elements_equal_except_image_data(
            self, report.html.html, expected_html
        )

    def test_create_pairwise_result_table(self) -> None:
        before_metric_name = "Test.Two.average"
        after_metric_name = "Test.One.average"
        before_test_name = "ui.OverviewPerfBefore"
        after_test_name = "ui.OverviewPerfAfter"

        samples = test_util.load_before_samples(
            before_test_name
        ) + test_util.load_after_samples(after_test_name)
        samples_by_id = test_util.samples_by_id(samples)

        experiment_cfg = analysis_cfg.ExperimentCfg(
            experiment_groups_cfgs=[
                analysis_cfg.ExperimentGroupsCfg(
                    metric_path_regex_list=[
                        f".*{before_test_name}\|{before_metric_name}",
                        f".*{after_test_name}\|{after_metric_name}",
                    ]
                )
            ]
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg(experiment_cfg=experiment_cfg)
        )
        results = [
            output.AnalysisResultForOutput(
                groups=groups_list[0],
                pairs=[
                    output.PairwiseResultForOutput(
                        result=analysis_results.PairwiseResult(
                            before=analysis_results.ExperimentGroup(
                                sample=samples_by_id[
                                    f"before|{before_test_name}|{before_metric_name}"
                                ],
                            ),
                            after=analysis_results.ExperimentGroup(
                                sample=samples_by_id[
                                    f"after|{after_test_name}|{after_metric_name}"
                                ],
                                bootstrap=stats_util.BootstrapResult(
                                    statistic_kind=stats_util.TestStatisticKind.MEAN,
                                    confidence_interval=stats_util.ConfidenceInterval(
                                        low=0, high=1, confidence=0.95
                                    ),
                                    bias_estimate=0,
                                ),
                            ),
                            hypothesis_result=stats_util.HypothesisTestResult(
                                statistic_kind=stats_util.TestStatisticKind.MEAN,
                                u=0.0,
                                p=1.0,
                            ),
                        ),
                        plots=[],
                    ),
                ],
                groups_plots=[],
            )
        ]
        report = html_report.HtmlReport(
            raw_test_results=[],
            results=results,
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        pair = results[0].pairs[0]
        table = report._create_pairwise_result_table(pair.result)

        expected_table = test_util.load_html(
            HTML_DIR / "pairwise_result_table.html"
        )
        test_util.assert_elements_equal_except_image_data(
            self, table, expected_table
        )

    def test_create_pairwise_result_figure(self) -> None:
        before_metric_name = "Test.One"
        after_metric_name = "Test.Three"
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

        experiment_cfg = analysis_cfg.ExperimentCfg(
            per_test_cfgs=[
                analysis_cfg.PerTestCfg(
                    test_name_regex="ui\\.OverviewPerf",
                    metric_name_regex_allowlist=[
                        rf"{before_metric_name}|{after_metric_name}"
                    ],
                )
            ]
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg(experiment_cfg=experiment_cfg)
        )
        pair = analysis_results.PairwiseResult(
            before=analysis_results.ExperimentGroup(
                sample=samples_by_id[
                    f"before|ui.OverviewPerf|{before_metric_name}.average"
                ]
            ),
            after=analysis_results.ExperimentGroup(
                sample=samples_by_id[
                    f"after|ui.OverviewPerf|{after_metric_name}.average"
                ]
            ),
            hypothesis_result=stats_util.HypothesisTestResult(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                u=0.0,
                p=1.0,
            ),
        )
        plot_data = plot.PlotData(
            kind=plot.PairwisePlotKind.PLOT_BOX, figure=figure.Figure()
        )
        results = [
            output.AnalysisResultForOutput(
                groups=groups_list[0],
                pairs=[
                    output.PairwiseResultForOutput(
                        result=pair, plots=[plot_data]
                    )
                ],
                groups_plots=[],
            )
        ]

        report = html_report.HtmlReport(
            raw_test_results=[],
            results=results,
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        figure_element = report._create_pairwise_result_figure(
            pair=pair, plot_data=plot_data
        )

        expected_figure = test_util.load_html(
            HTML_DIR / "pairwise_result_figure.html"
        )
        test_util.assert_elements_equal_except_image_data(
            self, figure_element, expected_figure
        )

    def test_append_groups_result_figure(self) -> None:
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        groups_list = (
            analysis_results._construct_explicit_experiment_groups_list(
                samples,
                cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        metric_path_regex_list=[
                            r"^ui\.OverviewPerf\|Test\.(One|Three)\.average$",
                        ]
                    )
                ],
            )
        )
        results = [
            output.AnalysisResultForOutput(
                groups=groups_list[0], pairs=[], groups_plots=[]
            ),
        ]

        plot_data = plot.PlotData(
            kind=plot.GroupsPlotKind.PLOT_BOX, figure=figure.Figure()
        )
        report = html_report.HtmlReport(
            raw_test_results=[],
            results=results,
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        report._append_groups_result_figure(
            groups_id=plot_util.get_groups_name_for_plot(groups_list[0]),
            plot_data=plot_data,
        )
        figure_element = report.html.body.findall("figure")[0]

        expected_figure = test_util.load_html(
            HTML_DIR / "groups_result_figure.html"
        )
        test_util.assert_elements_equal_except_image_data(
            self, figure_element, expected_figure
        )

    def test_embed_raw_data(self) -> None:
        raw_test_results = [
            test_result.TestResults(
                results={
                    test_result.TestResultKey(
                        run_id="run1",
                        test_name="test1",
                        metric_name="metric1",
                        variant="variant1",
                        label="before",
                    ): test_result.TestResult(
                        units="units1",
                        improvement_direction=test_result.ImprovementDirection.UP,
                        value=1,
                    ),
                    test_result.TestResultKey(
                        run_id="run2",
                        test_name="test2",
                        metric_name="metric2",
                        variant="variant2",
                        label="after",
                    ): test_result.TestResult(
                        units="units2",
                        improvement_direction=test_result.ImprovementDirection.DOWN,
                        value=2,
                    ),
                }
            )
        ]
        report = html_report.HtmlReport(
            raw_test_results=raw_test_results,
            results=[],
            template_dir=TEMPLATE_DIR,
            cfg=analysis_cfg.AnalysisCfg(),
        )
        report._embed_raw_data()
        loaded_test_results = test_result.TestResults.from_html(f"{report}")
        self.assertEqual(raw_test_results, loaded_test_results)
