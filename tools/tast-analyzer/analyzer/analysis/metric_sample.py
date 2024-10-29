# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from collections.abc import Iterator
import dataclasses
import itertools
import math

from analyzer.backend.test_result import ImprovementDirection
import numpy as np
import scipy


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class MetricSample:
    """Represents an aggregation of a particular metric from a particular test
    run over a set of test runs."""

    label: str
    """A label describing the experiment this came from."""

    sample_id: str
    """A unique identifier for this sample.

    For example, control_group|ui.OverviewPerf|Memory.Total.TileMemory.summary"""

    test_name: str
    """The name of a test this metric is from, e.g. ui.OverviewPerf."""

    metric_name: str
    """The name of this metric, e.g. Memory.Total.TileMemory.summary"""

    metric_path: str
    """Full metric path, e.g. ui.OverviewPerf|Memory.Total.TileMemory.summary"""

    units: str
    """Units of the metric value - e.g. 's' for seconds."""

    improvement_direction: ImprovementDirection
    """Whether this metric is better if it goes up or down."""

    _value_map: dict[str, list[float]] = dataclasses.field(default_factory=dict)
    """Map from test run ID to a list of values."""

    def values(self) -> Iterator[float]:
        return itertools.chain(*self._value_map.values())

    def size(self) -> int:
        """Returns the sample size of this MetricSample."""
        return sum(len(v) for v in self._value_map.values())

    def mean(self) -> float:
        """Returns the mean of the values in this MetricSample."""
        s = 0.0
        for v in self.values():
            s += v
        return s / self.size()

    def std(self) -> float:
        """Returns the standard deviation of the values in this MetricSample."""
        vals = list(self.values())
        return float(np.std(vals))

    def description(self, print_vals: bool = False) -> str:
        """Returns a human readable description of this MetricSample.

        Args:
            print_vals: Whether to print the values in the description.

        Returns:
            A string describing the distribution of values.
        """
        vals = list(self.values())
        s = ""
        if print_vals:
            s = "  " + " ".join(f"{i:.3g}" for i in vals) + "\n"
        d = scipy.stats.describe(vals)
        s += f"  mean={d.mean:.2f} {self.units}, std={math.sqrt(d.variance):.2f}, "
        s += f"min={d.minmax[0]:.2f}, max={d.minmax[1]:.2f}, skew={d.skewness:.2f}"
        return s

    def to_dict(self) -> dict:
        return dataclasses.asdict(self)
