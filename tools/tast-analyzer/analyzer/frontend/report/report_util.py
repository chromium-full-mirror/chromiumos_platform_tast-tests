# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import pathlib

from analyzer.analysis import analysis_cfg
from analyzer.backend import test_result
from analyzer.frontend import output
from analyzer.frontend.report import html_report
from analyzer.frontend.report import report_kind


def create_reports(
    *,
    raw_test_results: list[test_result.TestResults],
    results: list[output.AnalysisResultForOutput],
    kinds: set[report_kind.ReportKind],
    template_dir: pathlib.Path,
    cfg: analysis_cfg.AnalysisCfg,
    output_dir: pathlib.Path,
) -> None:
    """Creates and saves reports for the given results and report kinds.

    Args:
        raw_data: The raw data before statistical processing.
        results: The results to report.
        kinds: The kinds of reports to create.
        template_dir: The directory to load the template from.
        cfg: The analysis configuration.
        output_dir: The directory to save the reports to.
    """

    for kind in kinds:
        if kind == report_kind.ReportKind.REPORT_HTML:
            report = html_report.HtmlReport(
                raw_test_results=raw_test_results,
                results=results,
                template_dir=template_dir,
                cfg=cfg,
            )
            report.make()
            (output_dir / "index.html").write_text(f"{report}")
        else:
            raise ValueError(f"Unknown report kind: {kind}")
