# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.


import dataclasses
import enum
from pathlib import Path

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.analysis import analyze_results
from analyzer.analysis.analysis_results import AnalysisResult
import click


class _CliAnalysis(enum.StrEnum):
    PRINT_BETTER_WORSE = "print_better_worse"
    PRINT_TEST_BREAKDOWN = "print_test_breakdown"
    PRINT_BY_PCT_CHANGE = "print_by_pct_change"
    PRINT_BY_T_STAT = "print_by_t_stat"
    PRINT_MEAN_PCT_CHANGE = "print_mean_pct_change"


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

    print(f"{len(results)} metrics, {len(better)} better, {len(worse)} worse")

    if _CliAnalysis.PRINT_BETTER_WORSE in analyses:
        print(f"{len(worse)} GOT WORSE FROM {s1_name} to {s2_name}")
        _print_results(worse)
        print()

        print(f"{len(better)} GOT BETTER FROM {s1_name} to {s2_name}")
        _print_results(better)
        print()

    # TODO: implement other analyses


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
def print_results(compare: list[Path]) -> None:
    clicfg = _CliFrontendCfg()
    results = analyze_results.analyze_results(
        compare[0], compare[1], clicfg.cfg
    )
    _compare_results(
        s1_name=compare[0].name,
        s2_name=compare[1].name,
        results=results,
        analyses=clicfg.analyses,
    )
