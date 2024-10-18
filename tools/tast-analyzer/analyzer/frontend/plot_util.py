# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.


import logging
import pathlib

from analyzer.analysis import analysis_results
from analyzer.frontend import output
from analyzer.frontend import plot
from matplotlib import figure
from matplotlib import pyplot as plt
import seaborn as sns


def init_plotting() -> None:
    """Initialize plotting."""
    sns.set_theme()
    sns.set(font_scale=1)


def _create_plot_data_for_pair(
    pair: analysis_results.PairwiseResult,
    fig: figure.Figure,
    kind: plot.PlotKind,
) -> plot.PlotData:
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

    return plot.PlotData(kind=kind, figure=fig)


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
    plot_kinds: set[plot.PlotKind],
) -> list[output.AnalysisResultForOutput]:
    """Creates plots for the given results and plot kinds.

    Args:
        results: The results to plot.
        plot_kinds: The kinds of plots to create.

    Returns:
        A list of analysis results with their output data.
    """
    logging.info("Creating plots...")
    results_for_output: list[output.AnalysisResultForOutput] = []
    for result in results:
        pairs_for_output: list[output.PairwiseResultForOutput] = []
        for pair in result.pairs:
            logging.info(f"Creating plots for {pair.identifier()}")
            pairwise_result_plots: list[plot.PlotData] = []
            for kind in plot_kinds:
                if kind == plot.PlotKind.PLOT_CDF:
                    fig = _plot_cdfs(pair)
                elif kind == plot.PlotKind.PLOT_BOX:
                    fig = _plot_box(pair)
                else:
                    raise ValueError(f"Unknown plot kind: {kind}")
                pairwise_result_plots.append(
                    _create_plot_data_for_pair(pair, fig, kind)
                )
            pairs_for_output.append(
                output.PairwiseResultForOutput(
                    result=pair, plots=pairwise_result_plots
                )
            )

        results_for_output.append(
            output.AnalysisResultForOutput(
                groups=result.groups, pairs=pairs_for_output
            )
        )

    return results_for_output


def save_plots(
    *,
    results_for_output: list[output.AnalysisResultForOutput],
    plot_kinds: set[plot.PlotKind],
    plot_dir: pathlib.Path,
) -> None:
    """Saves plots in the given map.

    Args:
        results_for_output: A list of AnalysisResultForOutput to save.
        plot_kinds: The kinds of plots to save.
        plot_dir: The directory to save the plots to.
    """
    logging.info("Saving plots...")
    plot_dir.mkdir(parents=True, exist_ok=True)
    for result in results_for_output:
        for pair in result.pairs:
            identifier = pair.result.identifier()
            for plot_data in pair.plots:
                if plot_data.kind in plot_kinds:
                    name = f"{identifier}_{plot_data.kind.value}"
                    save_path = plot_dir.joinpath(f"{name}.png")
                    plot_data.figure.savefig(save_path, bbox_inches="tight")
