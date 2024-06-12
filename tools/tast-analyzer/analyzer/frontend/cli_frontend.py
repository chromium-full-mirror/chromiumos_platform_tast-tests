# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.


import dataclasses
import enum
from pathlib import Path
import statistics

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.analysis import analyze_results
from analyzer.analysis.analysis_results import AnalysisResult
import click


class _CliAnalysis(enum.StrEnum):
    PRINT_TEST_BREAKDOWN = "print_test_breakdown"
    PRINT_BY_PCT_CHANGE = "print_by_pct_change"
    PRINT_MEDIAN_PCT_CHANGE = "print_median_pct_change"


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


def _print_results(results: list[AnalysisResult]) -> None:
    """Prints a human readable summary of the analysis results."""
    for r in results:
        print(r.summary())


def _compare_results(
    *,
    s1_name: str,
    s2_name: str,
    results: list[analysis_results.AnalysisResult],
    analyses: list[_CliAnalysis],
) -> None:
    better, worse = analysis_results.split_better_and_worse_by_mean(results)

    print(f"Comparison from {s1_name} to {s2_name}:")
    print(f"{len(results)} metrics, {len(better)} better, {len(worse)} worse")

    def mean_change_key(r: analysis_results.AnalysisResult) -> float:
        return r.mean_change_better()

    if _CliAnalysis.PRINT_BY_PCT_CHANGE in analyses:
        better_by_pct_change = sorted(better, key=mean_change_key, reverse=True)
        worse_by_pct_change = sorted(worse, key=mean_change_key)
        print(f"{len(better_by_pct_change)} better by %change of mean")
        _print_results(better_by_pct_change)
        print()
        print(f"{len(worse_by_pct_change)} worse by %change of mean")
        _print_results(worse_by_pct_change)
        print()

    if _CliAnalysis.PRINT_TEST_BREAKDOWN in analyses:
        test_names = sorted(set(r.test_name() for r in results))
        better_by_test = {
            name: [r for r in better if r.test_name() == name]
            for name in test_names
        }
        worse_by_test = {
            name: [r for r in worse if r.test_name() == name]
            for name in test_names
        }

        for test_name in test_names:
            print("Better for", test_name)
            better = sorted(
                better_by_test[test_name], key=mean_change_key, reverse=True
            )
            _print_results(better)
            print()
            print("Worse for", test_name)
            worse = sorted(worse_by_test[test_name], key=mean_change_key)
            _print_results(worse)
            print()
            print()

    if _CliAnalysis.PRINT_MEDIAN_PCT_CHANGE in analyses:
        change_better = [r.mean_change_better() for r in results]
        median = statistics.median(change_better)
        print(f"Median improvement in mean: {100.0*median:.2}%")


@click.command()
@click.option(
    "-c",
    "--compare",
    type=click.Path(
        exists=True, dir_okay=False, resolve_path=True, path_type=Path
    ),
    help="stats tests",
    nargs=2,
    required=True,
)
@click.option(
    "-a",
    "--analyses",
    type=click.Choice(list(_CliAnalysis)),
    help="analyses to run",
    default=[_CliAnalysis.PRINT_TEST_BREAKDOWN],
    multiple=True,
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
    "-p",
    "--alpha-value",
    type=float,
    help="statistical significance level to use",
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
def print_results(
    compare: list[Path],
    analyses: list[_CliAnalysis],
    skip_all_zero: bool,
    minimum_sample_size: int,
    alpha_value: float,
    multiple_test_correction: analysis_cfg.MultipleTestCfg,
    metric_include_regex: str | None,
    metric_exclude_regex: str | None,
    remove_outliers: bool,
) -> None:
    cfg = analysis_cfg.AnalysisCfg(
        skip_all_zero_samples=skip_all_zero,
        minimum_sample_size=minimum_sample_size,
        alpha=alpha_value,
        multiple_test_cfg=multiple_test_correction,
        metric_exclude_regex=metric_exclude_regex,
        metric_include_regex=metric_include_regex,
        remove_outliers=remove_outliers,
    )

    clicfg = _CliFrontendCfg(cfg=cfg, analyses=analyses)
    results = analyze_results.analyze_results(
        compare[0], compare[1], clicfg.cfg
    )
    _compare_results(
        s1_name=compare[0].name,
        s2_name=compare[1].name,
        results=results,
        analyses=clicfg.analyses,
    )
