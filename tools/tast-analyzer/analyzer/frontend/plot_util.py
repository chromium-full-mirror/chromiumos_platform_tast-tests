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


def init_plotting() -> None:
    """Initialize plotting."""
    sns.set_theme()
    sns.set(font_scale=1)


def _save_figure_for_result(
    s1_name: str,
    s2_name: str,
    result: analysis_results.AnalysisResult,
    path: pathlib.Path,
    fig: figure.Figure,
    kind: PlotKind,
) -> None:
    direction = "higher" if result.is_up_better() else "lower"
    which_better = "after" if result.mean_change_better() > 0 else "before"
    fig.suptitle(
        f"{result.metric_path()}\n{direction} is better - {which_better} is better",
        wrap=True,
    )
    fig.tight_layout()
    name = f"{s1_name}_{s2_name}_{result.metric_path()}_{kind.value}"
    fig.savefig(f"{path}/{name}.png", bbox_inches="tight")


def _plot_cdfs(result: analysis_results.AnalysisResult) -> figure.Figure:
    fig, ax = plt.subplots()
    before_values = result.before_sample.value_map.values()
    after_values = result.after_sample.value_map.values()
    sns.ecdfplot(data={"before": before_values, "after": after_values}, ax=ax)
    ax.set_xlabel(result.units())
    return fig


def create_plots(
    *,
    s1_name: str,
    s2_name: str,
    results: list[analysis_results.AnalysisResult],
    plots: list[PlotKind],
    plot_dir: pathlib.Path,
) -> None:
    """Creates and saves plots for the given results and plot kinds.

    Args:
        s1_name: The name of the first sample.
        s2_name: The name of the second sample.
        results: The results to plot.
        plots: The kinds of plots to create.
        plot_dir: The directory to save the plots to.
    """
    logging.info(f"Creating plots for {s1_name} and {s2_name}")
    plot_dir.mkdir(parents=True, exist_ok=True)
    for result in results:
        logging.info(f"Creating plots for {result.metric_path()}")
        for plot_kind in plots:
            assert (
                plot_kind in PlotKind.PLOT_CDF
            ), f"Unknown plot kind: {plot_kind}"
            fig = _plot_cdfs(result)
            _save_figure_for_result(
                s1_name, s2_name, result, plot_dir, fig, plot_kind
            )
