# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import dataclasses
import enum
import json
import re

from analyzer.analysis import stats_util


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class PerTestCfg:
    """PerTestCfg holds configuration that may apply to a specific test.

    For example, blocking or allowing certain metrics within a particular test.
    Metrics are by default blocked from analysis if a PerTestCfg is specified.
    PerTestCfgs are specified by giving a regex that matches the test name.
    If multiple PerTestCfgs apply to a single test, then the PerTestCfgs are
    merged. The blocklist takes precedence over the allowlist. This can be
    used to block certain metrics in a subtest, for example."""

    test_name_regex: str = ""
    """Regex for identifying which tests this PerTestCfg applies to.

    For a merged PerTestCfg, this will be empty.
    """

    metric_name_regex_blocklist: list[str] = dataclasses.field(
        default_factory=list
    )
    """List of regex to block a metric name from analysis.

    This overrides any setting in the allowlist."""

    metric_name_regex_allowlist: list[str] = dataclasses.field(
        default_factory=list
    )
    """List of regex to allow a metric name in the analysis."""

    def metric_allowed(self, metric_name: str) -> bool:
        """Returns true if the metric name is allowed to be in the analysis."""
        for regex in self.metric_name_regex_blocklist:
            if re.match(regex, metric_name):
                return False
        for regex in self.metric_name_regex_allowlist:
            if re.match(regex, metric_name):
                return True
        return False

    def merge(self, other: "PerTestCfg") -> "PerTestCfg":
        """Returns a merged PerTestCfg.

        Args:
            other: PerTestCfg to merge with this PerTestCfg

        Returns:
            A merged PerTestCfg.
        """
        metric_name_regex_blocklist = list(
            set(
                self.metric_name_regex_blocklist
                + other.metric_name_regex_blocklist
            )
        )
        metric_name_regex_allowlist = list(
            set(
                self.metric_name_regex_allowlist
                + other.metric_name_regex_allowlist
            )
        )
        return PerTestCfg(
            test_name_regex="",
            metric_name_regex_blocklist=metric_name_regex_blocklist,
            metric_name_regex_allowlist=metric_name_regex_allowlist,
        )


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class ExperimentGroupsCfg:
    """Describes how to build a set of experiment groups to compare.

    Currently, only one of the below regex lists may be specified per
    ExperimentGroupsCfg."""

    metric_path_regex_list: list[str] = dataclasses.field(default_factory=list)
    """List of regexes to match metric paths.

    All metric paths matching any of the regexes will be considered in the same
    analysis as different experiment groups and compared to each other."""

    test_name_regex_list: list[str] = dataclasses.field(default_factory=list)
    """List of regexes to match test names.

    This will produce one list of experiment groups for each sample whose test
    name is matched by a regex in this list for each possible metric name."""


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class ExperimentCfg:
    per_test_cfgs: list[PerTestCfg] = dataclasses.field(default_factory=list)
    """List of PerTestCfg."""

    experiment_groups_cfgs: list[ExperimentGroupsCfg] | None = None
    """List of ExperimentGroupsCfg, defining experiment groups.

    If this is None, experiment groups will be defined for each metric path
    across MetricSamples identified by their label, i.e. a comparison between
    the same metric and test between differently labeled sets of results."""

    def compute_per_test_cfg(self, test_name: str) -> PerTestCfg:
        """Computes the effective PerTestCfg for the given test.

        Args:
            test_name: Name of test.

        Returns:
            A (potentially merged) PerTestCfg for the given test.
        """
        per_test_cfg: PerTestCfg | None = None
        for cfg in self.per_test_cfgs:
            if re.match(cfg.test_name_regex, test_name):
                per_test_cfg = per_test_cfg.merge(cfg) if per_test_cfg else cfg
        return (
            per_test_cfg
            if per_test_cfg
            else PerTestCfg(metric_name_regex_allowlist=["^.*$"])
        )

    @classmethod
    def from_json(cls, s: str) -> "ExperimentCfg":
        """Loads a PersistentCfg from JSON.

        Args:
            s: String containing the JSON.

        Returns:
            A PersistentCfg object.
        """
        d = json.loads(s)
        if "per_test_cfgs" in d:
            d["per_test_cfgs"] = [PerTestCfg(**v) for v in d["per_test_cfgs"]]
        if "experiment_groups_cfgs" in d:
            d["experiment_groups_cfgs"] = [
                ExperimentGroupsCfg(**v) for v in d["experiment_groups_cfgs"]
            ]
        return ExperimentCfg(**d)


class MultipleTestCfg(enum.StrEnum):
    """Configuration for how to handle ensemble statistical testing.

    We need to test many metrics against each-other. In general,
    tast-analyzer errs on the side of avoiding false positives at the expense
    of false negatives (i.e. you may miss a change, but you won't get a
    spurious change reported). Due to the nature of the Tast benchmarks, metrics
    are often non-independent (positively or negatively correlated). For
    example, improving performance is likely to make many metrics go up, so
    they are positively correlated.

    Controlling for FWER is useful if you want to minimize the number of false
    positives as much as possible. Controlling for FDR is useful if you are
    doing a more explorative comparison and want to see more things that might
    have improved or got worse.
    """

    FWER = "fwer"
    """Holm-Bonferroni procedure for controlling FWER.

    Use this for guaranteeing that the chance of one or more false positives is
    less than the significance level AnalysisCfg.alpha.

    Assumptions: None. """

    FDR = "fdr"
    """Benjamini-Yekutieli procedure for controlling FDR.

    Use this for guaranteeing that the rate of false positives is the
    significance level AnalysisCfg.alpha.

    Assumptions: None. """

    NONE = "none"
    """Perform no multiple test correction.

    This is not recommended."""

    def scipy_name(self) -> str:
        """Gets the name used by SciPy for the multiple test procedure."""
        if self == self.FWER:
            return "holm"
        elif self == self.FDR:
            return "fdr_by"
        else:
            raise ValueError(f"Unknown MultipleTestCfg for scipy: {self}")


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class AnalysisCfg:
    skip_all_zero_samples: bool = True
    """Whether to skip all zero samples in the analysis."""

    minimum_sample_size: int = 1
    """The minimum sample size required to include a sample in the analysis."""

    alpha: float = 0.05
    """The significance level for the analysis.

    No pruning is performed if this is negative."""

    hypothesis_test_params: stats_util.HypothesisTestParameters = (
        dataclasses.field(default_factory=stats_util.HypothesisTestParameters)
    )
    """The parameters for the hypothesis test."""

    bootstrap_params: stats_util.BootstrapParameters = dataclasses.field(
        default_factory=stats_util.BootstrapParameters
    )
    """The parameters for the bootstrap test."""

    multiple_test_cfg: MultipleTestCfg = MultipleTestCfg.FWER
    """The multiple test procedure to use."""

    metric_include_regex: str | None = None
    """Filter analyzed metrics to only those that match this regex."""

    metric_exclude_regex: str | None = None
    """Filter analyzed metrics to only those that do not match this regex.
    Excluding overrides including."""

    remove_outliers: bool = False
    """Whether to remove outlier values.

    This uses a simple strategy of removing one maximum and one minimum value
    from each sample."""

    experiment_cfg: ExperimentCfg = dataclasses.field(
        default_factory=ExperimentCfg
    )

    control_label: str | None = None
    """The label of the control group.

    If specified, the control group is compared against all other groups.
    Otherwise, all groups are compared against each other.
    """
