# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import dataclasses
from enum import StrEnum
import json


class ImprovementDirection(StrEnum):
    UP = "up"
    DOWN = "down"

    def is_up_better(self) -> bool:
        return self == "up"


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class TestResultKey:
    run_id: str
    """Unique run ID for this test run."""

    test_name: str
    """Test name from Tast - e.g. ui.OverviewPerf."""

    metric_name: str
    """Metric name from Tast - e.g. Memory.Total.TileMemory."""

    variant: str
    """Variant name from Tast - usually 'summary'."""

    label: str = ""
    """Label describing the experiment this came from.

    From results version: 2
    Compatibility: Set to the filename if not specified."""

    def sample_id(self) -> str:
        """Returns a unique identifier for which sample this test result should
        belong to."""
        return self.label + "." + self.sample_metric_path()

    def sample_metric_name(self) -> str:
        """Returns the name for the metric in the context of a set of test
        runs."""
        if not self.variant:
            return self.metric_name
        return self.metric_name + "." + self.variant

    def sample_metric_path(self) -> str:
        """Returns an identifier for the metric in the context of a set
        of test runs."""
        return self.test_name + "." + self.sample_metric_name()

    def to_json(self) -> str:
        return json.dumps(dataclasses.asdict(self), sort_keys=True)

    @classmethod
    def from_json(cls, s: str) -> "TestResultKey":
        return TestResultKey(**json.loads(s))


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class TestResult:
    units: str
    """Units of the metric value - e.g. 's' for seconds."""

    improvement_direction: ImprovementDirection
    """Whether this value is better if it goes up or down."""

    value: int | float | list[float]
    """The value of the test result."""

    def to_dict(self) -> dict:
        return dataclasses.asdict(self)

    @classmethod
    def from_dict(cls, d: dict) -> "TestResult":
        d["improvement_direction"] = ImprovementDirection(
            d["improvement_direction"]
        )
        return TestResult(**d)


METADATA_VERSION_CURRENT = 1
RESULTS_VERSION_CURRENT = 2


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class TestResultsMetadata:
    results_version: int = RESULTS_VERSION_CURRENT
    metadata_version: int = METADATA_VERSION_CURRENT

    @classmethod
    def from_dict(cls, d: dict) -> "TestResultsMetadata":
        return TestResultsMetadata(**d)


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class TestResults:
    metadata: TestResultsMetadata = dataclasses.field(
        default_factory=TestResultsMetadata
    )
    results: dict[TestResultKey, TestResult] = dataclasses.field(
        default_factory=dict
    )

    def merge(self, results: "TestResults") -> None:
        for key in results.results:
            assert key not in self.results, f"duplicate key: {key}"
        assert (
            self.metadata == results.metadata
        ), f"cannot merge test results with differing metadata: {self.metadata} != {results.metadata}"
        self.results.update(results.results)

    def to_json(self, indent: int = 2) -> str:
        d = {
            "metadata": dataclasses.asdict(self.metadata),
            "results": {
                k.to_json(): v.to_dict() for k, v in self.results.items()
            },
        }
        return json.dumps(d, sort_keys=True, indent=indent)

    @classmethod
    def from_json(cls, s: str) -> "TestResults":
        """Loads a previously ingested JSON performance results file.

        Args:
            s: String containing the json.

        Returns:
            A TestResults object.
        """
        d = json.loads(s)
        d["results"] = {
            TestResultKey.from_json(k): TestResult.from_dict(v)
            for k, v in d["results"].items()
        }
        d["metadata"] = TestResultsMetadata.from_dict(d["metadata"])
        return TestResults(**d)
