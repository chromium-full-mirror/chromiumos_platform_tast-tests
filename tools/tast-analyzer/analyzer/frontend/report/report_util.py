# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import enum
import pathlib

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.frontend import plot_util
from analyzer.frontend.report import html_report


class ReportKind(enum.StrEnum):
    """Kind of report to create."""

    REPORT_HTML = "report-html"
    """Creates an HTML report."""


def create_reports(
    *,
    results: list[analysis_results.AnalysisResult],
    reports: set[ReportKind],
    template_dir: pathlib.Path,
    identifier_to_plots_map: dict[str, list[plot_util.PlotData]],
    cfg: analysis_cfg.AnalysisCfg,
    output_dir: pathlib.Path,
) -> None:
    """Creates and saves reports for the given results and report kinds.

    Args:
        results: The results to report.
        reports: The kinds of reports to create.
        template_dir: The directory to load the template from.
        identifier_to_plots_map: The mapping from pairwise result identifiers
            to their plots.
        cfg: The analysis configuration.
        output_dir: The directory to save the reports to.
    """

    for report_kind in reports:
        if report_kind == ReportKind.REPORT_HTML:
            report = html_report.HtmlReport(
                results=results,
                template_dir=template_dir,
                cfg=cfg,
                identifier_to_plots_map=identifier_to_plots_map,
            )
            report.make()
            report.write(output_dir=output_dir)
        else:
            raise ValueError(f"Unknown report kind: {report_kind}")
