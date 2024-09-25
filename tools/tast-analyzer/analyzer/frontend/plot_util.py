# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

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


def init_plotting() -> None:
    """Initialize plotting."""
    sns.set_theme()
    sns.set(font_scale=1)


def _save_figure_for_pair(
    pair: analysis_results.PairwiseResult,
    path: pathlib.Path,
    fig: figure.Figure,
    kind: PlotKind,
) -> None:
    direction = "higher" if pair.is_up_better() else "lower"
    which_better = "after" if pair.mean_change_better() > 0 else "before"
    fig.suptitle(
        f"{pair.identifier()}\n{direction} is better - {which_better} is better",
        wrap=True,
    )
    fig.tight_layout()
    name = f"{pair.identifier()}_{kind.value}"
    fig.savefig(f"{path}/{name}.png", bbox_inches="tight")


def _plot_cdfs(pair: analysis_results.PairwiseResult) -> figure.Figure:
    fig, ax = plt.subplots()
    before_values = pair.before.sample.value_map.values()
    after_values = pair.after.sample.value_map.values()
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
    before_values = pair.before.sample.value_map.values()
    after_values = pair.after.sample.value_map.values()
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
    plot_dir: pathlib.Path,
) -> None:
    """Creates and saves plots for the given results and plot kinds.

    Args:
        results: The results to plot.
        plots: The kinds of plots to create.
        plot_dir: The directory to save the plots to.
    """
    logging.info("Creating plots...")
    plot_dir.mkdir(parents=True, exist_ok=True)
    for result in results:
        for pair in result.pairs:
            logging.info(f"Creating plots for {pair.identifier()}")
            for plot_kind in plots:
                if plot_kind == PlotKind.PLOT_CDF:
                    fig = _plot_cdfs(pair)
                elif plot_kind == PlotKind.PLOT_BOX:
                    fig = _plot_box(pair)
                else:
                    raise ValueError(f"Unknown plot kind: {plot_kind}")
                _save_figure_for_pair(pair, plot_dir, fig, plot_kind)
