# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
from collections import defaultdict
import logging
from pathlib import Path

from analyzer.analysis.metric_sample import MetricSample
from analyzer.backend.results import load_test_result_dict_from_json
from analyzer.backend.results import TestResult
from analyzer.backend.results import TestResultKey
import click


def _load_metrics_from_results_dict(
    results_dict: dict[TestResultKey, TestResult],
) -> dict[str, MetricSample]:
    metrics: dict[str, MetricSample] = {}

    logging.info(f"Examining {len(results_dict)} records")
    for key, result in sorted(results_dict.items()):
        val: float
        if isinstance(result.value, float):
            val = result.value
        elif isinstance(result.value, int):
            val = float(result.value)
        elif isinstance(result.value, list):
            # TODO(b/343114458): Consider using the entire sample somehow.
            # Currently, if there are multiple values we just take the mean.
            # Handling these as a sample is likely required to properly handle
            # TPS CUJ tests.
            val = sum(result.value) / len(result.value)
        else:
            assert False, f"Unknown value type: {result.value}"

        metric_path = key.metric_path()
        m = metrics.setdefault(
            metric_path,
            MetricSample(
                test_name=key.test_name,
                metric_path=metric_path,
                units=result.units,
                improvement_direction=result.improvement_direction,
                value_map={},
            ),
        )

        assert m.test_name == key.test_name
        assert m.metric_path == metric_path
        assert m.units == result.units
        assert m.improvement_direction == result.improvement_direction
        assert key.run_id not in m.value_map
        m.value_map[key.run_id] = val

    logging.info(f"Loaded {len(metrics)} metrics")
    sample_sizes: dict[int, int] = defaultdict(int)
    for v in metrics.values():
        sample_size = len(v.value_map)
        sample_sizes[sample_size] += 1
    logging.info(f"Sample size distribution: {sorted(sample_sizes.items())}")

    return metrics


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
def analyze_results(compare: list[Path]):
    before_results = load_test_result_dict_from_json(compare[0].read_text())
    after_results = load_test_result_dict_from_json(compare[1].read_text())
    before_metrics = _load_metrics_from_results_dict(before_results)
    after_metrics = _load_metrics_from_results_dict(after_results)

    # TODO: Do the analysis.
