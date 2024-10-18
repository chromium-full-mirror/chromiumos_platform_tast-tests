# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import dataclasses
import enum

from analyzer.analysis import analysis_results
from analyzer.frontend import plot
from analyzer.frontend.report import report_kind


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class PairwiseResultForOutput:
    """Holds a pairwise result and related output data."""

    result: analysis_results.PairwiseResult
    """The pairwise result."""

    plots: list[plot.PlotData]
    """A list of plot data of the pairwise result."""


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class AnalysisResultForOutput:
    """Holds a analysis result with output data."""

    groups: list[analysis_results.ExperimentGroup]
    """A list of groups in the analysis that have been compared to each other."""

    pairs: list[PairwiseResultForOutput]
    """A list of all pairwise results with their output data in the analysis."""


class OutputKind(enum.StrEnum):
    """Kind of output to create.

    This Enum is a combination of PlotKind and ReportKind as if it inherited
    them. This is because enum inheritance is not supported in Python.
    """

    PLOT_CDF = plot.PlotKind.PLOT_CDF
    """Plots the cumulative distribution function."""

    PLOT_BOX = plot.PlotKind.PLOT_BOX
    """Plots the box plot."""

    REPORT_HTML = report_kind.ReportKind.REPORT_HTML
    """Creates a summary report in HTML."""


def sort_output_kind(
    kinds: list[OutputKind],
) -> tuple[set[plot.PlotKind], set[report_kind.ReportKind]]:
    """Sorts a list of OutputKind into two separate sets, one containing
    PlotKind members and the other containing ReportKind members.

    Args:
        kinds: The list of OutputKind to sort.

    Returns:
        A tuple of two sets of PlotKind and ReportKind.
    """
    plot_kinds: set[plot.PlotKind] = set()
    report_kinds: set[report_kind.ReportKind] = set()

    for kind in kinds:
        if kind == OutputKind.PLOT_CDF:
            plot_kinds.add(plot.PlotKind.PLOT_CDF)
        elif kind == OutputKind.PLOT_BOX:
            plot_kinds.add(plot.PlotKind.PLOT_BOX)
        elif kind == OutputKind.REPORT_HTML:
            report_kinds.add(report_kind.ReportKind.REPORT_HTML)
        else:
            ValueError(f"Unknown output kind: {kind}")

    return plot_kinds, report_kinds
