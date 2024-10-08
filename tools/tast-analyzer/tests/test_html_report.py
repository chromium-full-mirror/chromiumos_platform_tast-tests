# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import pathlib
import tempfile
import unittest
import xml.etree.ElementTree as ET

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.analysis import stats_util
from analyzer.frontend.report import html_report
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
            analysis_results.AnalysisResult(
                groups=groups_list[0],
                pairs=[
                    analysis_results.PairwiseResult(
                        before=analysis_results.ExperimentGroup(
                            sample=samples_by_id[
                                f"before.{before_test_name}.Test.One.average"
                            ]
                        ),
                        after=analysis_results.ExperimentGroup(
                            sample=samples_by_id[
                                f"after.{after_test_name}.Test.One.average"
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
        ]

        report = html_report.HtmlReport(
            results, TEMPLATE_DIR, analysis_cfg.AnalysisCfg()
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
            analysis_results.AnalysisResult(
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
        ]
        report = html_report.HtmlReport(
            results, TEMPLATE_DIR, analysis_cfg.AnalysisCfg()
        )
        self.assertEqual(report._labels(), ["after", "before"])

    def test_metric_paths(self) -> None:
        before_metric_name = "Test.One.average"
        after_metric_name = "Test.Three.average"
        before_metric_path = f"ui.OverviewPerf.{before_metric_name}"
        after_metric_path = f"ui.OverviewPerf.{after_metric_name}"
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
            analysis_results.AnalysisResult(
                groups=groups_list[0],
                pairs=[
                    analysis_results.PairwiseResult(
                        before=analysis_results.ExperimentGroup(
                            sample=samples_by_id[f"before.{before_metric_path}"]
                        ),
                        after=analysis_results.ExperimentGroup(
                            sample=samples_by_id[f"after.{after_metric_path}"]
                        ),
                        hypothesis_result=stats_util.HypothesisTestResult(
                            statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                            u=0.0,
                            p=1.0,
                        ),
                    )
                ],
            )
        ]
        report = html_report.HtmlReport(
            results, TEMPLATE_DIR, analysis_cfg.AnalysisCfg()
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
            analysis_results.AnalysisResult(
                groups=groups_list[0],
                pairs=[
                    analysis_results.PairwiseResult(
                        before=analysis_results.ExperimentGroup(
                            sample=samples_by_id[
                                f"before.{before_test_name}.Test.One.average"
                            ]
                        ),
                        after=analysis_results.ExperimentGroup(
                            sample=samples_by_id[
                                f"after.{after_test_name}.Test.One.average"
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
        ]
        report = html_report.HtmlReport(
            results, TEMPLATE_DIR, analysis_cfg.AnalysisCfg()
        )
        report._set_title()

        expected_html = self._load_html(HTML_DIR / "set_title.html")
        self._assert_elements_equal(report.html.html, expected_html)

    def test_create_sample_size_table(self) -> None:
        metric_path = "ui.OverviewPerf.Test.One.average"

        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg()
        )
        results = [
            analysis_results.AnalysisResult(
                groups=groups_list[0],
                pairs=[
                    analysis_results.PairwiseResult(
                        before=analysis_results.ExperimentGroup(
                            sample=samples_by_id[f"before.{metric_path}"]
                        ),
                        after=analysis_results.ExperimentGroup(
                            sample=samples_by_id[f"after.{metric_path}"]
                        ),
                        hypothesis_result=stats_util.HypothesisTestResult(
                            statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                            u=0.0,
                            p=1.0,
                        ),
                    )
                ],
            )
        ]
        report = html_report.HtmlReport(
            results, TEMPLATE_DIR, analysis_cfg.AnalysisCfg()
        )
        table = report._create_sample_size_table()

        expected_table = self._load_html(HTML_DIR / "sample_size_table.html")
        self._assert_elements_equal(table, expected_table)

    def test_append_summary_empty_results(self) -> None:
        report = html_report.HtmlReport(
            [], TEMPLATE_DIR, analysis_cfg.AnalysisCfg()
        )
        report._append_summary()

        expected_html = self._load_html(
            HTML_DIR / "append_summary_empty_results.html"
        )
        self._assert_elements_equal(report.html.html, expected_html)

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
                sample=samples_by_id["before.ui.OverviewPerf.Test.One.average"]
            ),
            after=analysis_results.ExperimentGroup(
                sample=samples_by_id["after.ui.OverviewPerf.Test.One.average"]
            ),
            hypothesis_result=stats_util.HypothesisTestResult(
                statistic_kind=stats_util.TestStatisticKind.RANK_SUM,
                u=0.0,
                p=1.0,
            ),
        )
        results = [
            analysis_results.AnalysisResult(groups=groups_list[0], pairs=[pair])
        ]
        report = html_report.HtmlReport(
            results, TEMPLATE_DIR, analysis_cfg.AnalysisCfg()
        )
        report._append_summary()

        expected_html = self._load_html(
            HTML_DIR / "append_summary_with_results.html"
        )
        self._assert_elements_equal(report.html.html, expected_html)

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
                        f".*{before_test_name}.{before_metric_name}",
                        f".*{after_test_name}.{after_metric_name}",
                    ]
                )
            ]
        )
        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg(experiment_cfg=experiment_cfg)
        )
        results = [
            analysis_results.AnalysisResult(
                groups=groups_list[0],
                pairs=[
                    analysis_results.PairwiseResult(
                        before=analysis_results.ExperimentGroup(
                            sample=samples_by_id[
                                f"before.{before_test_name}.{before_metric_name}"
                            ],
                        ),
                        after=analysis_results.ExperimentGroup(
                            sample=samples_by_id[
                                f"after.{after_test_name}.{after_metric_name}"
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
                    )
                ],
            )
        ]
        report = html_report.HtmlReport(
            results, TEMPLATE_DIR, analysis_cfg.AnalysisCfg()
        )
        pair = results[0].pairs[0]
        table = report._create_pairwise_result_table(pair)

        expected_table = self._load_html(
            HTML_DIR / "pairwise_result_table.html"
        )
        self._assert_elements_equal(table, expected_table)

    def test_write(self) -> None:
        samples = (
            test_util.load_before_samples() + test_util.load_after_samples()
        )
        samples_by_id = test_util.samples_by_id(samples)

        groups_list = analysis_results.construct_experiment_groups_list(
            samples, analysis_cfg.AnalysisCfg()
        )
        results = [
            analysis_results.AnalysisResult(
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
        ]
        report = html_report.HtmlReport(
            results, TEMPLATE_DIR, analysis_cfg.AnalysisCfg()
        )

        with tempfile.TemporaryDirectory() as temp:
            export_dir = pathlib.Path(temp)
            report.write(output_dir=export_dir)
            self.assertTrue(export_dir.joinpath("index.html").exists())

    def _load_html(self, path: pathlib.Path) -> ET.Element:
        return ET.fromstring(path.read_text())

    def _assert_html_text_equal(
        self, text1: str | None, text2: str | None
    ) -> None:
        # Consecutive whitespace is normalized to a single space in HTML
        text1 = text1 = " ".join(text1.split()) if text1 else ""
        text2 = text2 = " ".join(text2.split()) if text2 else ""
        self.assertEqual(text1, text2)

    def _assert_elements_equal(
        self, element1: ET.Element, element2: ET.Element
    ) -> None:
        self.assertEqual(element1.tag, element2.tag)
        self._assert_html_text_equal(element1.text, element2.text)
        self.assertDictEqual(element1.attrib, element2.attrib)

        for child1, child2 in zip(element1, element2, strict=True):
            self._assert_elements_equal(child1, child2)
