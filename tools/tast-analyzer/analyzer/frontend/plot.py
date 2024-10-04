# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import dataclasses
import enum

from matplotlib import figure


class PairwisePlotKind(enum.StrEnum):
    """Kind of pairwise plot to create."""

    PLOT_CDF = "pairwise-plot-cdf"
    """Plots the cumulative distribution function."""

    PLOT_BOX = "pairwise-plot-box"
    """Plots the box plot."""

    def description(self) -> str:
        """Returns the description of this PairwisePlotKind."""
        if self == PairwisePlotKind.PLOT_CDF:
            return "Empirical Cumulative Distribution Function (eCDF)"
        if self == PairwisePlotKind.PLOT_BOX:
            return "Box plot"
        raise ValueError("Unknown plot kind")


class GroupsPlotKind(enum.StrEnum):
    """Kind of groups plot to create.

    eCDF, which is supported as a pairwise plot, is not supported because groups
    level eCDF plots are unlikely to be useful because many lines are overlapped
    with each other.
    """

    PLOT_BOX = "groups-plot-box"
    """Plots the box plot."""

    def description(self) -> str:
        """Returns the description of this groups."""
        if self == GroupsPlotKind.PLOT_BOX:
            return "Box plot"
        raise ValueError("Unknown plot kind")


PlotKind = PairwisePlotKind | GroupsPlotKind
"""Available plot kind."""


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class PlotData:
    kind: PlotKind
    """The plot kind of this data."""

    figure: figure.Figure
    """The figure of the plot."""
