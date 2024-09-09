# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import pathlib
import tempfile
import unittest

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

        report = html_report.HtmlReport(results, TEMPLATE_DIR)
        self.assertEqual(
            report._test_names(), [after_test_name, before_test_name]
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
        report = html_report.HtmlReport(results, TEMPLATE_DIR)

        title = f"A/B Test Report | {after_test_name}, {before_test_name}"
        self.assertEqual(report.html.title.text, title)

        self.assertEqual(len(report.html.body), 1)
        h1 = report.html.body[0]
        self.assertEqual(h1.tag, "h1")
        self.assertEqual(h1.text, title)

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
        report = html_report.HtmlReport(results, TEMPLATE_DIR)

        with tempfile.TemporaryDirectory() as temp:
            export_dir = pathlib.Path(temp)
            report.write(output_dir=export_dir)
            self.assertTrue(export_dir.joinpath("index.html").exists())
