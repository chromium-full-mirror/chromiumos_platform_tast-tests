# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import dataclasses
import enum

from matplotlib import figure


class PlotKind(enum.StrEnum):
    """Kind of plot to create."""

    PLOT_CDF = "plot-cdf"
    """Plots the cumulative distribution function."""

    PLOT_BOX = "plot-box"
    """Plots the box plot."""

    def description(self) -> str:
        """Returns the description of this PlotKind."""
        if self == PlotKind.PLOT_CDF:
            return "Empirical Cumulative Distribution Function (eCDF)"
        if self == PlotKind.PLOT_BOX:
            return "Box plot"
        raise ValueError("Unknown plot kind")


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class PlotData:
    kind: PlotKind
    """The plot kind of this data."""

    figure: figure.Figure
    """The figure of the plot."""
