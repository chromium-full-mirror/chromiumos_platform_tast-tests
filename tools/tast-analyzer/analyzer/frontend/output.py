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

    groups_plots: list[plot.PlotData]
    """A list of groups level plot data."""


class OutputKind(enum.StrEnum):
    """Kind of output to create.

    This Enum is a combination of PairwisePlotKind, GroupsPlotKind, and ReportKind
    as if it inherited them. This is because enum inheritance is not supported in Python.
    """

    PAIRWISE_PLOT_CDF = plot.PairwisePlotKind.PLOT_CDF
    """Plots the cumulative distribution function."""

    PAIRWISE_PLOT_BOX = plot.PairwisePlotKind.PLOT_BOX
    """Plots the box plot."""

    GROUPS_PLOT_BOX = plot.GroupsPlotKind.PLOT_BOX
    """Plots the box plot."""

    REPORT_HTML = report_kind.ReportKind.REPORT_HTML
    """Creates a summary report in HTML."""


def sort_output_kind(
    kinds: list[OutputKind],
) -> tuple[
    set[plot.PairwisePlotKind],
    set[plot.GroupsPlotKind],
    set[report_kind.ReportKind],
]:
    """Sorts a list of OutputKind into a tuple of three sets of PairwisePlotKind,
    GroupsPlotKind, and ReportKind.

    Args:
        kinds: The list of OutputKind to sort.

    Returns:
        A tuple of three sets of PairwisePlotKind, GroupsPlotKind, and ReportKind.
    """
    pairwise_plot_kinds: set[plot.PairwisePlotKind] = set()
    groups_plot_kinds: set[plot.GroupsPlotKind] = set()
    report_kinds: set[report_kind.ReportKind] = set()

    for kind in kinds:
        if kind == OutputKind.PAIRWISE_PLOT_CDF:
            pairwise_plot_kinds.add(plot.PairwisePlotKind.PLOT_CDF)
        elif kind == OutputKind.PAIRWISE_PLOT_BOX:
            pairwise_plot_kinds.add(plot.PairwisePlotKind.PLOT_BOX)
        elif kind == OutputKind.GROUPS_PLOT_BOX:
            groups_plot_kinds.add(plot.GroupsPlotKind.PLOT_BOX)
        elif kind == OutputKind.REPORT_HTML:
            report_kinds.add(report_kind.ReportKind.REPORT_HTML)
        else:
            ValueError(f"Unknown output kind: {kind}")

    return pairwise_plot_kinds, groups_plot_kinds, report_kinds
