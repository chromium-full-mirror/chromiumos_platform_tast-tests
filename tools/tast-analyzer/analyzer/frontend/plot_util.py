# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import dataclasses
import enum
import logging
import pathlib

from analyzer.analysis import analysis_results
from matplotlib import figure
from matplotlib import pyplot as plt
import seaborn as sns


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


def init_plotting() -> None:
    """Initialize plotting."""
    sns.set_theme()
    sns.set(font_scale=1)


def _create_plot_data_for_pair(
    pair: analysis_results.PairwiseResult,
    fig: figure.Figure,
    kind: PlotKind,
) -> PlotData:
    """Creates PlotData for the given pairwise result.

    Args:
        pair: The pairwise result to plot onto the figure.
        fig: The figure to put into the PlotData.
        kind: The plot kind of the figure.

    Returns:
        A PlotData.
    """
    direction = "higher" if pair.is_up_better() else "lower"
    which_better = "after" if pair.mean_change_better() > 0 else "before"
    fig.suptitle(
        f"{pair.identifier()}\n{direction} is better - {which_better} is better",
        wrap=True,
    )
    fig.tight_layout()

    return PlotData(kind=kind, figure=fig)


def _plot_cdfs(pair: analysis_results.PairwiseResult) -> figure.Figure:
    fig, ax = plt.subplots()
    before_values = pair.before.sample.values()
    after_values = pair.after.sample.values()
    sns.ecdfplot(
        data={
            pair.before.label(): before_values,
            pair.after.label(): after_values,
        },
        ax=ax,
    )
    ax.set_xlabel(pair.units())
    return fig


def _plot_box(pair: analysis_results.PairwiseResult) -> figure.Figure:
    fig, ax = plt.subplots()
    before_values = list(pair.before.sample.values())
    after_values = list(pair.after.sample.values())
    sns.boxplot(
        data={
            pair.before.label(): before_values,
            pair.after.label(): after_values,
        },
        color=(0.9, 0.9, 0.9, 0.9),
        ax=ax,
    )
    sns.stripplot(
        data={
            pair.before.label(): before_values,
            pair.after.label(): after_values,
        },
        ax=ax,
    )
    ax.set_ylabel(pair.units())
    return fig


def create_plots(
    *,
    results: list[analysis_results.AnalysisResult],
    plots: set[PlotKind],
) -> dict[str, list[PlotData]]:
    """Creates plots for the given results and plot kinds.

    Args:
        results: The results to plot.
        plots: The kinds of plots to create.

    Returns:
        A mapping from pairwise result identifiers to their plot data.
    """
    logging.info("Creating plots...")
    pairwise_result_plots_map: dict[str, list[PlotData]] = {}
    for result in results:
        for pair in result.pairs:
            logging.info(f"Creating plots for {pair.identifier()}")
            pairwise_result_plots: list[PlotData] = []
            for plot_kind in plots:
                if plot_kind == PlotKind.PLOT_CDF:
                    fig = _plot_cdfs(pair)
                elif plot_kind == PlotKind.PLOT_BOX:
                    fig = _plot_box(pair)
                else:
                    raise ValueError(f"Unknown plot kind: {plot_kind}")
                pairwise_result_plots.append(
                    _create_plot_data_for_pair(pair, fig, plot_kind)
                )
            pairwise_result_plots_map[pair.identifier()] = pairwise_result_plots

    return pairwise_result_plots_map


def save_plots(
    *,
    identifier_to_plots_map: dict[str, list[PlotData]],
    plots: set[PlotKind],
    plot_dir: pathlib.Path,
) -> None:
    """Saves plots in the given map.

    Args:
        identifier_to_plots_map: A mapping from result identifiers to their
            plot data.
        plots: The kinds of plots to save.
        plot_dir: The directory to save the plots to.
    """
    logging.info("Saving plots...")
    plot_dir.mkdir(parents=True, exist_ok=True)
    for identifier, plot_data_list in identifier_to_plots_map.items():
        for plot_data in plot_data_list:
            if plot_data.kind in plots:
                name = f"{identifier}_{plot_data.kind.value}"
                save_path = plot_dir.joinpath(f"{name}.png")
                plot_data.figure.savefig(save_path, bbox_inches="tight")
