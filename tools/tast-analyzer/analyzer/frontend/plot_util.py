# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.


import collections
import logging
import pathlib

from analyzer.analysis import analysis_results
from analyzer.frontend import output
from analyzer.frontend import plot
from matplotlib import figure
from matplotlib import pyplot as plt
import numpy as np
import seaborn as sns


def init_plotting() -> None:
    """Initialize plotting."""
    sns.set_theme()
    sns.set(font_scale=1)


def _get_groups_name_for_plot(
    groups: list[analysis_results.ExperimentGroup],
) -> str:
    """Returns a name of the given groups for groups level plots."""
    labels_by_metric_path = collections.defaultdict(list)
    for group in groups:
        labels_by_metric_path[group.metric_path()].append(group.label())

    metric_paths_with_label: list[str] = []
    for metric_path, labels in labels_by_metric_path.items():
        metric_name = "|".join(labels)
        if len(labels) > 1:
            metric_name = f"({metric_name})"
        metric_paths_with_label.append(f"{metric_name}|{metric_path}")

    return ", ".join(metric_paths_with_label)


def _create_plot_data_for_groups(
    result: analysis_results.AnalysisResult,
    fig: figure.Figure,
    kind: plot.GroupsPlotKind,
) -> plot.PlotData:
    """Creates the figure for the given groups.

    Args:
        results: The result to create PlotData for.
        fig: The figure to create.
        kind: The plot kind of the figure.

    Returns:
        A PlotData.
    """
    groups_name = _get_groups_name_for_plot(result.groups)
    direction = "higher" if result.is_up_better() else "lower"
    fig.suptitle(f"{groups_name}\n{direction} is better", wrap=True)
    fig.tight_layout()

    return plot.PlotData(kind=kind, figure=fig)


def _plot_box_for_groups(
    result: analysis_results.AnalysisResult,
) -> figure.Figure:
    """Creates the box plot for groups.

    Args:
        results: The result to plot.

    Returns:
        The created figure.
    """
    label_to_values = {
        group.label(): list(group.sample.values()) for group in result.groups
    }

    fig, ax = plt.subplots()
    order = sorted(label_to_values, key=lambda x: np.mean(label_to_values[x]))
    sns.boxplot(
        data=label_to_values, color=(0.9, 0.9, 0.9, 0.9), ax=ax, order=order
    )
    sns.stripplot(data=label_to_values, ax=ax, order=order)
    ax.set_ylabel(result.units())

    return fig


def _create_plots_for_groups(
    result: analysis_results.AnalysisResult,
    plot_kinds: set[plot.GroupsPlotKind],
) -> list[plot.PlotData]:
    """Creates groups level plots for the given result.

    Args:
        results: The result to plot.
        plot_kinds: The kinds of plots to create.

    Returns:
        A list of PlotData.
    """
    groups_plot: list[plot.PlotData] = []
    for kind in plot_kinds:
        if kind == plot.GroupsPlotKind.PLOT_BOX:
            fig = _plot_box_for_groups(result)
        else:
            raise ValueError(f"Unknown plot kind: {kind}")
        groups_plot.append(
            _create_plot_data_for_groups(result=result, fig=fig, kind=kind)
        )
    return groups_plot


def _create_plot_data_for_pair(
    pair: analysis_results.PairwiseResult,
    fig: figure.Figure,
    kind: plot.PairwisePlotKind,
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


def _plot_cdfs_for_pair(pair: analysis_results.PairwiseResult) -> figure.Figure:
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


def _plot_box_for_pair(pair: analysis_results.PairwiseResult) -> figure.Figure:
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
    pairwise_plot_kinds: set[plot.PairwisePlotKind],
    groups_plot_kinds: set[plot.GroupsPlotKind],
) -> list[output.AnalysisResultForOutput]:
    """Creates plots for the given results and plot kinds.

    Args:
        results: The results to plot.
        pairwise_plot_kinds: The kinds of pairwise plots to create.
        groups_plot_kinds: The kinds of groups plots to create.

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
            for kind in pairwise_plot_kinds:
                if kind == plot.PairwisePlotKind.PLOT_CDF:
                    fig = _plot_cdfs_for_pair(pair)
                elif kind == plot.PairwisePlotKind.PLOT_BOX:
                    fig = _plot_box_for_pair(pair)
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
                groups=result.groups,
                pairs=pairs_for_output,
                groups_plots=_create_plots_for_groups(
                    result, groups_plot_kinds
                ),
            )
        )

    return results_for_output


def _save_plot(
    identifier: str, plot_data: plot.PlotData, plot_dir: pathlib.Path
) -> None:
    """Saves a plot to the given directory.

    Args:
        identifier: The identifier of the plot.
        plot_data: The plot data to save.
        plot_dir: The directory to save the plot to.
    """
    name = f"{identifier}_{plot_data.kind.value}"
    save_path = plot_dir.joinpath(f"{name}.png")
    plot_data.figure.savefig(save_path, bbox_inches="tight")


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
                    _save_plot(identifier, plot_data, plot_dir)
        for plot_data in result.groups_plots:
            if plot_data.kind in plot_kinds:
                _save_plot(
                    get_groups_name_for_plot(result.groups), plot_data, plot_dir
                )
