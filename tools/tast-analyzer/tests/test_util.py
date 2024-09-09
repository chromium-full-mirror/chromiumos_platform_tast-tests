# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import pathlib

from analyzer.analysis import analyze_results
from analyzer.analysis import metric_sample
from analyzer.backend import tast_results_dir


FILES_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute().joinpath("files")
)


def load_before_samples(
    test_name: str = "ui.OverviewPerf",
) -> list[metric_sample.MetricSample]:
    """Loads samples for the before group.

    Args:
        test_name: Test name of the samples.

    Returns:
        A list of loaded samples.
    """
    results = tast_results_dir._load_results_from_results_chart_json(
        path=pathlib.Path(f"/before/tests/{test_name}/results-chart.json"),
        json_str=FILES_DIR.joinpath("results-chart-analysis1.json").read_text(),
        label="before",
    )
    return analyze_results._load_samples_from_test_results(results)


def load_after_samples(
    test_name: str = "ui.OverviewPerf",
) -> list[metric_sample.MetricSample]:
    """Loads samples for the after group.

    Args:
        test_name: Test name of the samples.

    Returns:
        A list of loaded samples.
    """
    results = tast_results_dir._load_results_from_results_chart_json(
        path=pathlib.Path(f"/after/tests/{test_name}/results-chart.json"),
        json_str=FILES_DIR.joinpath("results-chart-analysis2.json").read_text(),
        label="after",
    )
    return analyze_results._load_samples_from_test_results(results)


def samples_by_id(
    samples: list[metric_sample.MetricSample],
) -> dict[str, metric_sample.MetricSample]:
    """Creates a map from sample id to sample.

    Args:
        samples: A list of samples to create a map for.

    Returns:
        A map from sample id to sample.
    """
    samples_by_id = {}
    for s in samples:
        assert s.sample_id not in samples_by_id
        samples_by_id[s.sample_id] = s
    return samples_by_id
