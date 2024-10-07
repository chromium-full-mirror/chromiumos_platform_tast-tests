# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.


import dataclasses
import enum
import logging
import pathlib
import statistics

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.analysis import analyze_results
from analyzer.analysis import stats_util
from analyzer.frontend import output
from analyzer.frontend import plot
from analyzer.frontend import plot_util
from analyzer.frontend.report import report_util
import click


class _CliAnalysis(enum.StrEnum):
    PRINT_TEST_BREAKDOWN = "print-test-breakdown"
    PRINT_BY_PCT_CHANGE = "print-by-pct-change"
    PRINT_MEDIAN_PCT_CHANGE = "print-median-pct-change"


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class _CliFrontendCfg:
    cfg: analysis_cfg.AnalysisCfg = dataclasses.field(
        default_factory=analysis_cfg.AnalysisCfg
    )
    """The configuration for the statistical analysis."""

    analyses: list[_CliAnalysis] = dataclasses.field(
        default_factory=lambda: list(_CliAnalysis)
    )
    """The CLI analyses to run."""


def _print_pairs(pairs: list[analysis_results.PairwiseResult]) -> None:
    """Prints a human readable summary of the pairwise results."""
    for pair in pairs:
        print(pair.summary())


def _compare_results(
    *,
    results: list[analysis_results.AnalysisResult],
    analyses: list[_CliAnalysis],
) -> None:
    pairs = [p for result in results for p in result.pairs]
    better, worse = analysis_results.split_better_and_worse_by_mean(pairs)

    print(f"{len(better)} comparisons better, {len(worse)} comparisons worse")

    def mean_change_key(pair: analysis_results.PairwiseResult) -> float:
        return pair.mean_change_better()

    if _CliAnalysis.PRINT_BY_PCT_CHANGE in analyses:
        better_by_pct_change = sorted(better, key=mean_change_key, reverse=True)
        worse_by_pct_change = sorted(worse, key=mean_change_key)
        print(f"{len(better_by_pct_change)} better by %change of mean")
        _print_pairs(better_by_pct_change)
        print()
        print(f"{len(worse_by_pct_change)} worse by %change of mean")
        _print_pairs(worse_by_pct_change)
        print()

    if _CliAnalysis.PRINT_TEST_BREAKDOWN in analyses:

        def test_names_str(p: analysis_results.PairwiseResult) -> str:
            return ",".join(p.test_names())

        test_names = sorted(set(test_names_str(p) for p in pairs))
        better_by_test = {
            names: [p for p in better if test_names_str(p) == names]
            for names in test_names
        }
        worse_by_test = {
            names: [p for p in worse if test_names_str(p) == names]
            for names in test_names
        }

        for test_name in test_names:
            print("Better for", test_name)
            better = sorted(
                better_by_test[test_name], key=mean_change_key, reverse=True
            )
            _print_pairs(better)
            print()
            print("Worse for", test_name)
            worse = sorted(worse_by_test[test_name], key=mean_change_key)
            _print_pairs(worse)
            print()
            print()

    if _CliAnalysis.PRINT_MEDIAN_PCT_CHANGE in analyses:
        change_better = [p.mean_change_better() for p in pairs]
        median = statistics.median(change_better) if change_better else 0.0
        print(f"Median improvement in mean: {100.0*median:.2f}%")


@click.command()
@click.option(
    "-a",
    "--analyses",
    type=click.Choice(list(_CliAnalysis)),
    help="analyses to run",
    default=[
        _CliAnalysis.PRINT_TEST_BREAKDOWN,
        _CliAnalysis.PRINT_MEDIAN_PCT_CHANGE,
    ],
    multiple=True,
)
@click.option(
    "--outputs",
    type=click.Choice(list(output.OutputKind)),
    help="outputs to generate",
    default=[],
    multiple=True,
)
@click.option(
    "--output-dir",
    type=click.Path(
        exists=False, file_okay=False, resolve_path=True, path_type=pathlib.Path
    ),
    help="directory to output artifacts (plots, report, etc.) in",
    required=False,
)
@click.option(
    "--skip-all-zero/--no-skip-all-zero",
    type=bool,
    help="whether to skip samples with all zero values",
    default=True,
)
@click.option(
    "--minimum-sample-size",
    type=int,
    help="minimum sample size to include in the analysis",
    default=1,
)
@click.option(
    "--statistic-kind",
    type=click.Choice(list(stats_util.TestStatisticKind)),
    help="test statistic to use - rank-sum will use Mann-Whitney U which is "
    "faster but less powerful",
    default=stats_util.TestStatisticKind.MEAN,
)
@click.option(
    "--resamples",
    type=int,
    help="number of resamples to use for resampling methods",
    default=stats_util.DEFAULT_RESAMPLING_COUNT,
)
@click.option(
    "--deterministic/--no-deterministic",
    type=bool,
    help="whether to use deterministic resampling",
    default=False,
)
@click.option(
    "--confidence",
    type=float,
    help="confidence interval to report for the test statistic [0, 1.0]",
    default=0.95,
)
@click.option(
    "-p",
    "--alpha-value",
    type=float,
    help="statistical significance level to use - none if negative",
    default=0.05,
)
@click.option(
    "-m",
    "--multiple-test-correction",
    type=click.Choice(list(analysis_cfg.MultipleTestCfg)),
    help="correction method to use for multiple tests",
    default=analysis_cfg.MultipleTestCfg.FWER,
)
@click.option(
    "--metric-include-regex",
    type=str,
    help="regex to include metric paths by",
    required=False,
)
@click.option(
    "--metric-exclude-regex",
    type=str,
    help="regex to exclude metric paths by",
    required=False,
)
@click.option(
    "--remove-outliers/--no-remove-outliers",
    type=bool,
    help="clip min and max values as outliers",
    default=False,
)
@click.option(
    "--experiment-cfg-path",
    type=click.Path(
        exists=True, dir_okay=False, resolve_path=True, path_type=pathlib.Path
    ),
    help="path to file containing experiment configuration",
    required=False,
)
@click.option(
    "--control-label",
    type=str,
    help="if specified, the group with this label is compared against all "
    "other groups",
    required=False,
)
@click.argument(
    "sample-paths",
    type=click.Path(
        exists=True, dir_okay=False, resolve_path=True, path_type=pathlib.Path
    ),
    required=True,
    nargs=-1,
)
def print_results(
    sample_paths: list[pathlib.Path],
    analyses: list[_CliAnalysis],
    outputs: list[output.OutputKind],
    output_dir: pathlib.Path | None,
    skip_all_zero: bool,
    minimum_sample_size: int,
    statistic_kind: stats_util.TestStatisticKind,
    resamples: int,
    deterministic: bool,
    confidence: float,
    alpha_value: float,
    multiple_test_correction: analysis_cfg.MultipleTestCfg,
    metric_include_regex: str | None,
    metric_exclude_regex: str | None,
    remove_outliers: bool,
    experiment_cfg_path: pathlib.Path | None,
    control_label: str | None,
) -> None:
    """Computes analysis from one or more JSON files containing samples."""
    experiment_cfg = (
        analysis_cfg.ExperimentCfg.from_json(experiment_cfg_path.read_text())
        if experiment_cfg_path
        else analysis_cfg.ExperimentCfg()
    )
    cfg = analysis_cfg.AnalysisCfg(
        skip_all_zero_samples=skip_all_zero,
        minimum_sample_size=minimum_sample_size,
        alpha=alpha_value,
        hypothesis_test_params=stats_util.HypothesisTestParameters(
            statistic_kind=statistic_kind,
            resamples=resamples,
            deterministic=deterministic,
        ),
        bootstrap_params=stats_util.BootstrapParameters(
            statistic_kind=statistic_kind,
            resamples=resamples,
            deterministic=deterministic,
            confidence=confidence,
        ),
        multiple_test_cfg=multiple_test_correction,
        metric_exclude_regex=metric_exclude_regex,
        metric_include_regex=metric_include_regex,
        remove_outliers=remove_outliers,
        experiment_cfg=experiment_cfg,
        control_label=control_label,
    )

    clicfg = _CliFrontendCfg(cfg=cfg, analyses=analyses)
    results = analyze_results.analyze_results(sample_paths, clicfg.cfg)
    _compare_results(
        results=results,
        analyses=clicfg.analyses,
    )

    if outputs:
        assert output_dir, "must specify an output directory for given outputs"

        (
            save_pairwise_plot_kinds,
            save_groups_plot_kinds,
            report_kinds,
        ) = output.sort_output_kind(outputs)

        # Currently, all available plots are used for the report
        report_pairwise_plot_kinds: set[plot.PairwisePlotKind] = (
            set(plot.PairwisePlotKind) if report_kinds else set()
        )
        report_groups_plot_kinds: set[plot.GroupsPlotKind] = (
            set(plot.GroupsPlotKind) if report_kinds else set()
        )

        logging.info("Creating plots (this may take a long time)...")
        plot_util.init_plotting()
        results_for_output = plot_util.create_plots(
            results=results,
            pairwise_plot_kinds=save_pairwise_plot_kinds
            | report_pairwise_plot_kinds,
            groups_plot_kinds=save_groups_plot_kinds | report_groups_plot_kinds,
            control_label=cfg.control_label,
        )
        plot_util.save_plots(
            results_for_output=results_for_output,
            plot_kinds=save_pairwise_plot_kinds | save_groups_plot_kinds,
            plot_dir=output_dir,
        )

        if report_kinds:
            # Meant to be the project root (tast-analyzer/)
            root = pathlib.Path(__file__).parent.parent.parent
            template_dir = root / "configs" / "report" / "templates"

            logging.info("Creating a summary report...")
            report_util.create_reports(
                results=results_for_output,
                kinds=report_kinds,
                template_dir=template_dir,
                cfg=cfg,
                output_dir=output_dir,
            )
