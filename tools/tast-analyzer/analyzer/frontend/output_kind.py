# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import enum

from analyzer.frontend import plot_util
from analyzer.frontend.report import report_util


class OutputKind(enum.StrEnum):
    """Kind of output to create.

    This Enum is a combination of PlotKind and ReportKind as if it inherited
    them. This is because enum inheritance is not supported in Python.
    """

    PLOT_CDF = plot_util.PlotKind.PLOT_CDF
    """Plots the cumulative distribution function."""

    PLOT_BOX = plot_util.PlotKind.PLOT_BOX
    """Plots the box plot."""

    REPORT_HTML = report_util.ReportKind.REPORT_HTML
    """Creates a summary report in HTML."""


def sort_output_kind(
    kinds: list[OutputKind],
) -> tuple[set[plot_util.PlotKind], set[report_util.ReportKind]]:
    """Sorts a list of OutputKind into two separate sets, one containing
    PlotKind members and the other containing ReportKind members.

    Args:
        kinds: The list of OutputKind to sort.

    Returns:
        A tuple of two sets of PlotKind and ReportKind.
    """
    plot_kinds: set[plot_util.PlotKind] = set()
    report_kinds: set[report_util.ReportKind] = set()

    for kind in kinds:
        if kind == OutputKind.PLOT_CDF:
            plot_kinds.add(plot_util.PlotKind.PLOT_CDF)
        elif kind == OutputKind.PLOT_BOX:
            plot_kinds.add(plot_util.PlotKind.PLOT_BOX)
        elif kind == OutputKind.REPORT_HTML:
            report_kinds.add(report_util.ReportKind.REPORT_HTML)
        else:
            ValueError(f"Unknown output kind: {kind}")

    return plot_kinds, report_kinds
