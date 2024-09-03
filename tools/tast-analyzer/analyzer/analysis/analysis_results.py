# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.


from collections import defaultdict
import dataclasses
import itertools
import logging

from analyzer.analysis import metric_sample
from analyzer.analysis import stats_util
from analyzer.analysis.metric_sample import MetricSample


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class ExperimentGroup:
    """ExperimentGroup holds data analysis data for a particular sample.

    During analysis, some number of ExperimentGroups are compared together. For
    example, the A and B in an A/B test would be mapped to two ExperimentGroups
    for each metric being compared."""

    sample: MetricSample
    """The sample for this experiment group."""

    bootstrap: stats_util.BootstrapResult | None = None
    """The result of running a bootstrap to compute confidence intervals
    on this group.

    This is initially None until AnalysisResults are generated, where it is
    set if the test statistic is supported for bootstrapping."""

    def confidence_desc(self) -> str:
        """Returns a human readable description of the confidence interval,
        if it exists."""
        if self.bootstrap is None:
            return ""

        kind = self.bootstrap.statistic_kind.value
        confidence = self.bootstrap.confidence_interval.confidence
        low = self.bootstrap.confidence_interval.low
        high = self.bootstrap.confidence_interval.high
        bias = self.bootstrap.bias_estimate

        return f", {kind} {100*confidence:.1f}%=[{low:.2f}, {high:.2f}], E[bias]={bias:.2f}"


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class PairwiseResult:
    before: ExperimentGroup
    """The first experiment group's results."""

    after: ExperimentGroup
    """The second experiment group's results."""

    hypothesis_result: stats_util.HypothesisTestResult
    """The result of running the hypothesis test."""

    def __post_init__(self) -> None:
        assert self.before.sample.units == self.after.sample.units
        assert (
            self.before.sample.improvement_direction
            == self.after.sample.improvement_direction
        )

    def mean_change_better(self) -> float:
        """Returns the proportion change by which the mean has gotten better."""
        change = stats_util.signed_change(
            self.before.sample.mean(), self.after.sample.mean()
        )
        if self.is_up_better():
            return change
        else:
            return -change

    def test_names(self) -> list[str]:
        """Returns a list of the test names in this result.

        This may contain more than one test name if experiment groups were
        explicitly set up to compare tests with different names."""
        names = [self.before.sample.test_name]
        if self.after.sample.test_name not in names:
            names.append(self.after.sample.test_name)
        return names

    def summary(self) -> str:
        """Returns a human readable summary of this result."""
        s = f"{self.identifier()}:\n"

        s += (
            f"  {self.hypothesis_result.summary()}, "
            f"dir={self.before.sample.improvement_direction}, "
            f"n=({len(self.before.sample.value_map)}, "
            f"{len(self.after.sample.value_map)}), "
            f"%better={100.0*self.mean_change_better():.2f}%\n"
        )

        s += f"{self.before.sample.description()}{self.before.confidence_desc()}\n"
        s += f"{self.after.sample.description()}{self.after.confidence_desc()}"
        return s

    def identifier(self) -> str:
        """Returns a human readable identifier for this result."""
        before_label = self.before.sample.label
        after_label = self.after.sample.label
        before_path = self.before.sample.metric_path
        after_path = self.after.sample.metric_path

        label_diff = before_label != after_label
        path_diff = before_path != after_path
        if label_diff and path_diff:
            return f"{before_label}{before_path}->{after_label}{after_path}"

        if label_diff:
            return f"{before_path}:{before_label}->{after_label}"

        if path_diff:
            return f"{before_path}->{after_path}"

        return before_path

    def units(self) -> str:
        """Returns the units of the quantity."""
        return self.before.sample.units

    def is_up_better(self) -> bool:
        """Returns if going up is better for this metric."""
        return self.before.sample.improvement_direction.is_up_better()


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class AnalysisResult:
    groups: list[ExperimentGroup]
    """A list of groups in the analysis that will be compared to each other."""

    pairs: list[PairwiseResult]
    """A list of pairwise results between all groups in the analysis."""

    def __post_init__(self) -> None:
        for group in self.groups:
            assert group.sample.units == self.groups[0].sample.units
            assert (
                group.sample.improvement_direction
                == self.groups[0].sample.improvement_direction
            )
        # There should be N choose 2 pairwise results.
        assert (
            len(self.pairs) == (len(self.groups) * (len(self.groups) - 1)) / 2
        )


def _construct_implicit_experiment_groups_list(
    samples: list[metric_sample.MetricSample],
) -> list[list[ExperimentGroup]]:
    """Constructs a list of list of ExperimentGroups based on metric path.

    This function creates ExperimentGroups for samples with the same metric
    path. For example, if there are 5 samples with the same metric path,
    a list of 5 ExperimentGroups will be created and returned as part of the
    experiment groups list.

    Args:
        samples: List of samples.

    Returns:
        A list of lists of ExperimentGroups.
    """
    groups_list = []
    groups_by_metric_path = defaultdict(list)

    for s in samples:
        groups_by_metric_path[s.metric_path].append(ExperimentGroup(sample=s))

    for groups in groups_by_metric_path.values():
        # Only use if there are enough groups for a comparison.
        if len(groups) > 1:
            groups_list.append(groups)

    return groups_list


def construct_experiment_groups_list(
    samples: list[metric_sample.MetricSample],
) -> list[list[ExperimentGroup]]:
    """Computes a list of lists of experiment groups to be compared.

    Generally speaking, each list of ExperimentGroups should have samples that
    are related - for example, their metric path is the same, meaning that
    the same metric would be compared across different experiments.

    Args:
        samples: List of samples.

    Returns:
        A list of lists of ExperimentGroups.
    """
    groups_list = _construct_implicit_experiment_groups_list(samples)

    logging.info(
        f"Looking at {len(groups_list)} collections of samples for comparison"
    )
    return sorted(groups_list, key=lambda x: x[0].sample.metric_path)


def generate_analysis_results(
    *,
    groups_list: list[list[ExperimentGroup]],
    hypothesis_params: stats_util.HypothesisTestParameters,
    bootstrap_params: stats_util.BootstrapParameters,
) -> list[AnalysisResult]:
    """Makes a list of AnalysisResults for the given list of list of groups.

    Args:
        groups_list: List of list of ExperimentGroup.
        hypothesis_params: Parameters for hypothesis testing.
        bootstrap_params: Parameters for bootstrapping.

    Returns:
        A list of analysis results.
    """
    out = []
    for groups in groups_list:
        # For each group, compute its confidence interval.
        for i in range(len(groups)):
            bootstrap = bootstrap_params.run_one_sample_bootstrap(
                groups[i].sample
            )
            groups[i] = dataclasses.replace(groups[i], bootstrap=bootstrap)

        # For each ordered pair of groups, compute the hypothesis test.
        pairs = []
        for before, after in itertools.combinations(groups, 2):
            hypothesis_result = hypothesis_params.run_hypothesis_test(
                before.sample, after.sample
            )

            pairs.append(
                PairwiseResult(
                    before=before,
                    after=after,
                    hypothesis_result=hypothesis_result,
                )
            )

        out.append(AnalysisResult(groups=groups, pairs=pairs))
    return out


def split_better_and_worse_by_mean(
    pairs: list[PairwiseResult],
) -> tuple[list[PairwiseResult], list[PairwiseResult]]:
    """Splits the given PairwiseResult into ones that got better and worse.

    If a result has no change, it is skipped.

    Args:
        pairs: A list of PairwiseResults.

    Returns:
        A tuple of two lists of better and worse PairwiseResults.
    """
    better = []
    worse = []
    for pair in pairs:
        pair.after.sample.mean()
        sign = pair.after.sample.mean() - pair.before.sample.mean()
        if sign == 0.0:
            logging.warn(
                f"Skipping comparison with no changes - {pair.identifier()}. "
                "This may mean a subset of the data is duplicated between the "
                "control and experiment groups (ingested data may not have "
                "been cleared between runs)."
            )
            continue
        went_up = sign > 0.0
        if went_up == pair.is_up_better():
            better.append(pair)
        else:
            worse.append(pair)
    return better, worse
