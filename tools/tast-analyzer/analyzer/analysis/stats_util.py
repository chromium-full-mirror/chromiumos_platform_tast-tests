# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import dataclasses
import enum
import logging

from analyzer.analysis import metric_sample
import numpy as np
import numpy.typing as npt
from scipy import stats


class TestStatisticKind(enum.StrEnum):
    MEAN = "mean"
    MEDIAN = "median"
    STDDEV = "stddev"
    RANK_SUM = "rank-sum"

    def compute_value(self, values: npt.NDArray, axis: int = -1) -> npt.NDArray:
        """Computes the statistic value for a single sample."""
        if self == TestStatisticKind.MEAN:
            return np.mean(values, axis=axis)
        if self == TestStatisticKind.MEDIAN:
            return np.median(values, axis=axis)
        if self == TestStatisticKind.STDDEV:
            return np.std(values, axis=axis)
        if self == TestStatisticKind.RANK_SUM:
            raise NotImplementedError()
        raise ValueError(f"Unknown test statistic kind: {self}")

    def compute_statistic(
        self, a: npt.NDArray, b: npt.NDArray, axis: int = -1
    ) -> npt.NDArray:
        """Computes the test statistic for two samples."""
        return self.compute_value(a, axis) - self.compute_value(b, axis)


@dataclasses.dataclass(kw_only=True, order=True)
class HypothesisTestResult:
    statistic_kind: TestStatisticKind
    """The test statistic used in this hypothesis test."""

    u: float
    """The test statistic value."""

    p: float
    """The p-value."""

    def summary(self) -> str:
        """Returns a human readable summary of this result."""
        return f"stat={self.statistic_kind.value}, u={self.u}, p={self.p:.6f}"


def mannwhitneyu_test(
    s1: metric_sample.MetricSample, s2: metric_sample.MetricSample
) -> HypothesisTestResult:
    """Computes the Mann-Whitney U-statistic between two metric samples.

    Args:
        s1: The first metric sample.
        s2: The second metric sample.

    Returns:
        A HypothesisTestResult with the U-statistic and p-value.
    """
    x = list(s1.value_map.values())
    y = list(s2.value_map.values())
    u, p = stats.mannwhitneyu(x, y, alternative="two-sided")
    return HypothesisTestResult(
        statistic_kind=TestStatisticKind.RANK_SUM, u=u, p=p
    )


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class HypothesisTestParameters:
    statistic_kind: TestStatisticKind = TestStatisticKind.MEAN
    """The test statistic used in this hypothesis test."""

    resamples: int = 99999
    """The number of resamples to use in the permutation test."""

    deterministic: bool = False
    """Whether to use a deterministic seed for the permutation test."""

    def run_hypothesis_test(
        self, s1: metric_sample.MetricSample, s2: metric_sample.MetricSample
    ) -> HypothesisTestResult:
        """Runs the hypothesis test.

        This may fall back to the Mann-Whitney U test if the sample size is
        not large enough.

        Args:
            s1: The first metric sample.
            s2: The second metric sample.

        Returns:
            A HypothesisTestResult.
        """
        if self.statistic_kind == TestStatisticKind.RANK_SUM:
            return mannwhitneyu_test(s1, s2)

        res = _permutation_test(s1, s2, self)
        if res is None:
            logging.warning(
                f"Failed to run permutation test for {s1.metric_path}, falling "
                "back to Mann-Whitney U test. This can happen if the sample "
                "size is not large enough."
            )
            return mannwhitneyu_test(s1, s2)

        return res


def _permutation_test(
    s1: metric_sample.MetricSample,
    s2: metric_sample.MetricSample,
    params: HypothesisTestParameters,
) -> HypothesisTestResult | None:
    """Performs a two-tailed permutation test.

    This can fail if there is not enough data.

    Args:
        s1: The first metric sample.
        s2: The second metric sample.
        params: The parameters for the test.

    Returns:
        A HypothesisTestResult if successful, or None if the test failed.
    """
    # We can't perform the permutation test without at least two values.
    if len(s1.value_map) < 2 or len(s2.value_map) < 2:
        return None

    s1_values = list(s1.value_map.values())
    s2_values = list(s2.value_map.values())
    seed = 0 if params.deterministic else None

    res = stats.permutation_test(
        [s1_values, s2_values],
        statistic=params.statistic_kind.compute_statistic,
        permutation_type="independent",
        n_resamples=params.resamples,
        alternative="two-sided",
        random_state=seed,
        vectorized=True,
    )

    return HypothesisTestResult(
        statistic_kind=params.statistic_kind,
        u=res.statistic,
        p=res.pvalue,
    )


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class ConfidenceInterval:
    low: float
    """The lower bound of the confidence interval."""

    high: float
    """The upper bound of the confidence interval."""

    confidence: float
    """The proportion of the distribution the confidence interval covers."""


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class BootstrapResult:
    statistic_kind: TestStatisticKind
    """The test statistic used in this bootstrap."""

    confidence_interval: ConfidenceInterval
    """The confidence interval.

    This is the confidence interval for the bootstrap distribution."""

    bias_estimate: float
    """Estimate of the bias.

    This is the mean of bootstrap distribution subtract observed test
    statistic."""


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class BootstrapParameters:
    statistic_kind: TestStatisticKind = TestStatisticKind.MEAN
    """The test statistic used in the bootstrap."""

    resamples: int = 99999
    """The number of resamples to use in the bootstrap."""

    deterministic: bool = False
    """Whether to use a deterministic seed for the bootstrap."""

    confidence: float = 0.95
    """The confidence level to compute in the bootstrap."""

    def run_one_sample_bootstrap(
        self, s: metric_sample.MetricSample
    ) -> BootstrapResult | None:
        """Runs a one-sample bootstrap to compute the confidence intervals.

        This can fail if there is not enough data.

        Args:
            s: The metric sample.

        Returns:
            A BootstrapResult if successful, or None if the bootstrap failed.
        """
        if self.statistic_kind == TestStatisticKind.RANK_SUM:
            return None

        return _one_sample_bootstrap(s, self)


def _one_sample_bootstrap(
    s: metric_sample.MetricSample,
    params: BootstrapParameters,
) -> BootstrapResult | None:
    """Performs a one-sample bootstrap to compute confidence intervals.

    This can fail if there is not enough data.

    Args:
        s: The metric sample.
        params: The parameters for the bootstrap.

    Returns:
        A BootstrapResult if successful, or None if the bootstrap failed.
    """
    x = list(s.value_map.values())

    # Want there to be at least 100 distinct resamples to avoid monte carlo
    # error. Really, it should be at least 1000 for confidence intervals
    # according to more recent literature.
    BOOTSTRAP_MIN_SAMPLE_SIZE = 5
    if len(x) < BOOTSTRAP_MIN_SAMPLE_SIZE:
        logging.warning(
            f"{s.metric_path} has sample size {len(x)}, which is less than "
            f"{BOOTSTRAP_MIN_SAMPLE_SIZE}. Your confidence intervals may be "
            "invalid for this test metric."
        )

    # We can't perform the bootstrap without at least two values.
    if len(x) < 2:
        return None

    RESAMPLE_SIZE_RECOMMENDATION = 99999
    if params.resamples < RESAMPLE_SIZE_RECOMMENDATION:
        logging.warning(
            f"Avoid using less than {RESAMPLE_SIZE_RECOMMENDATION} resamples "
            "for accurate results."
        )

    seed = 0 if params.deterministic else None
    res = stats.bootstrap(
        [x],
        statistic=params.statistic_kind.compute_value,
        method="bca",
        n_resamples=params.resamples,
        confidence_level=params.confidence,
        random_state=seed,
        vectorized=True,
    )

    observed_statistic = params.statistic_kind.compute_value(np.array(x))
    bootstrap_statistic = np.mean(res.bootstrap_distribution)
    bias_estimate = bootstrap_statistic - observed_statistic

    return BootstrapResult(
        statistic_kind=params.statistic_kind,
        confidence_interval=ConfidenceInterval(
            low=res.confidence_interval.low,
            high=res.confidence_interval.high,
            confidence=params.confidence,
        ),
        bias_estimate=bias_estimate,
    )


def signed_change(before: float, after: float) -> float:
    """Returns the signed change proportion between two values.

    Args:
        before: The value before the change.
        after: The value after the change.

    Returns:
        The signed change as a proportion.
    """
    return (after - before) / before
