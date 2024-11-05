# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
from collections import defaultdict
import copy
import dataclasses
import logging
import pathlib
import re

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.analysis import metric_sample
from analyzer.backend import test_result
from statsmodels.stats import multitest


def _convert_value(val: int | float) -> float:
    if isinstance(val, float):
        return val
    if isinstance(val, int):
        return float(val)
    assert False, f"Unknown value type: {val}"


def load_test_results(
    paths: list[pathlib.Path],
) -> list[test_result.TestResults]:
    """Loads a list of test results from the given paths.

    Args:
        paths: A list of paths to the test results JSON files.

    Returns:
        A list of TestResults.
    """
    test_results: list[test_result.TestResults] = []
    for path in paths:
        results = test_result.TestResults.from_json(path.read_text())

        # Set labels to the filename if the results version is old.
        if results.metadata.results_version < 2:
            migrated_results = {}
            for result_key, result_value in results.results.items():
                assert result_key.label == ""
                migrated_key = dataclasses.replace(result_key, label=path.name)
                migrated_results[migrated_key] = result_value
            results = dataclasses.replace(results, results=migrated_results)

        test_results.append(results)

    return test_results


def _load_samples_from_test_results(
    results: test_result.TestResults,
) -> list[metric_sample.MetricSample]:
    samples_by_id: dict[str, metric_sample.MetricSample] = {}

    logging.info(f"Examining {len(results.results)} records")
    for key, result in sorted(results.results.items()):
        s = samples_by_id.setdefault(
            key.sample_id(),
            metric_sample.MetricSample(
                label=key.label,
                sample_id=key.sample_id(),
                test_name=key.test_name,
                metric_name=key.sample_metric_name(),
                metric_path=key.sample_metric_path(),
                units=result.units,
                improvement_direction=result.improvement_direction,
            ),
        )

        assert s.label == key.label
        assert s.sample_id == key.sample_id()
        assert s.test_name == key.test_name
        assert s.metric_path == key.sample_metric_path()
        assert s.units == result.units
        assert s.improvement_direction == result.improvement_direction
        assert key.run_id not in s._value_map

        if isinstance(result.value, list):
            values = [_convert_value(v) for v in result.value]

            # We don't have a way to determine if lists of scalar values are a
            # time series or not. These can't be compared like a regular sample.
            # They also tend to be large, and that can slow down processing a
            # lot. Heuristically detect this case and take the arithmetic mean.
            LARGE_TEST_RESULT_LIMIT = 64
            if len(values) > LARGE_TEST_RESULT_LIMIT:
                logging.warning(
                    f"Sample {s.sample_id} has a test result with many"
                    f" ({len(values)}) values. This may be a time-series and "
                    "results for it won't be valid for this sample if so. "
                    "Taking the mean to avoid long computation time."
                )
                values = [sum(values) / len(values)]
            s._value_map[key.run_id] = values
        else:
            s._value_map[key.run_id] = [_convert_value(result.value)]

    logging.info(f"Loaded {len(samples_by_id)} samples")
    sample_sizes: dict[int, int] = defaultdict(int)
    for s in samples_by_id.values():
        sample_sizes[s.size()] += 1
    logging.info(f"Sample size distribution: {sorted(sample_sizes.items())}")

    return list(samples_by_id.values())


def load_samples_from_test_results(
    test_results: list[test_result.TestResults],
) -> list[metric_sample.MetricSample]:
    all_samples: list[metric_sample.MetricSample] = []
    sample_ids: set[str] = set()
    for results in test_results:
        samples = _load_samples_from_test_results(results)
        for sample in samples:
            # Warn if there are multiple samples with the same ID. This could
            # happen if the same file is passed twice, or if the file is an
            # older format and does not contain the label and the file-name
            # is the same.
            if sample.sample_id in sample_ids:
                logging.warn(
                    f"Sample with duplicate ID ({sample.sample_id}) detected. "
                    "This will be treated as a separate sample. This can "
                    "happen if your results JSON files have not been updated "
                    "to include a label for each test result. Try regenerating "
                    "your results JSON files from the source data."
                )
            all_samples.append(sample)
            sample_ids.add(sample.sample_id)

    return all_samples


def _prune_non_significant_results(
    results: list[analysis_results.AnalysisResult],
    cfg: analysis_cfg.AnalysisCfg,
) -> list[analysis_results.AnalysisResult]:
    """Prune results that are not significant."""
    if cfg.multiple_test_cfg == analysis_cfg.MultipleTestCfg.NONE:
        return results

    # multitest.multipletests does not work if there are zero results.
    if not results:
        return results

    p_values = []
    for result in results:
        for pair in result.pairs:
            p_values.append(pair.hypothesis_result.p)

    # If the alpha value is negative, do not prune results but still compute
    # adjusted p-values.
    prune = cfg.alpha >= 0.0
    alpha = cfg.alpha if prune else 1.0
    rejects, p_corrected, _, _ = multitest.multipletests(
        p_values, alpha=alpha, method=cfg.multiple_test_cfg.scipy_name()
    )

    idx = 0
    out_samples = []
    for result in results:
        out_pairs = []
        for pair in result.pairs:
            # Reject the null hypothesis (that they are the same).
            if rejects[idx] or not prune:
                out_pair = copy.deepcopy(pair)
                out_pair.hypothesis_result.p = p_corrected[idx]
                out_pairs.append(out_pair)
            idx += 1
        if out_pairs:
            out_samples.append(dataclasses.replace(result, pairs=out_pairs))

    return out_samples


def _prune_regex_include(
    samples: list[metric_sample.MetricSample], regex: str
) -> list[metric_sample.MetricSample]:
    return [s for s in samples if re.search(regex, s.metric_path)]


def _prune_regex_exclude(
    samples: list[metric_sample.MetricSample], regex: str
) -> list[metric_sample.MetricSample]:
    return [s for s in samples if not re.search(regex, s.metric_path)]


def _prune_experiment_cfg(
    samples: list[metric_sample.MetricSample], cfg: analysis_cfg.ExperimentCfg
) -> list[metric_sample.MetricSample]:
    out_samples = []
    for s in samples:
        test_cfg = cfg.compute_per_test_cfg(s.test_name)

        if test_cfg.metric_allowed(s.metric_name):
            out_samples.append(s)
    return out_samples


def _prune_outliers(
    samples: list[metric_sample.MetricSample],
) -> list[metric_sample.MetricSample]:
    out_samples = []
    for s in samples:
        sorted_vals = sorted(s.values())
        value_map = copy.deepcopy(s._value_map)
        outliers = [sorted_vals[0], sorted_vals[-1]] if sorted_vals else []
        # Remove an arbitrary instance of each outlier.
        for lst in value_map.values():
            for outlier in outliers[:]:
                if outlier in lst:
                    lst.remove(outlier)
                    outliers.remove(outlier)
        # Remove runs that no longer have any values:
        value_map = {k: v for k, v in value_map.items() if v}
        out_samples.append(dataclasses.replace(s, _value_map=value_map))
    return out_samples


def _prune_all_zero_samples(
    samples: list[metric_sample.MetricSample],
) -> list[metric_sample.MetricSample]:
    out_samples = []
    for s in samples:
        if any(i != 0.0 for i in s.values()):
            out_samples.append(s)
    return out_samples


def _prune_minimum_sample_size(
    samples: list[metric_sample.MetricSample],
    sample_size: int,
) -> list[metric_sample.MetricSample]:
    return [s for s in samples if s.size() >= sample_size]


def analyze_results(
    samples: list[metric_sample.MetricSample],
    cfg: analysis_cfg.AnalysisCfg,
) -> list[analysis_results.AnalysisResult]:
    """Returns AnalysisResults for the given saved sample data paths."""
    if cfg.remove_outliers:
        samples = _prune_outliers(samples)

    samples = _prune_experiment_cfg(samples, cfg.experiment_cfg)

    if cfg.metric_include_regex:
        samples = _prune_regex_include(samples, cfg.metric_include_regex)

    if cfg.metric_exclude_regex:
        samples = _prune_regex_exclude(samples, cfg.metric_exclude_regex)

    # Skip any things with just zeros - seems to happen for broken tests.
    if cfg.skip_all_zero_samples:
        samples = _prune_all_zero_samples(samples)

    samples = _prune_minimum_sample_size(samples, cfg.minimum_sample_size)

    groups_list = analysis_results.construct_experiment_groups_list(
        samples, cfg
    )
    results = analysis_results.generate_analysis_results(
        groups_list=groups_list, cfg=cfg
    )

    return _prune_non_significant_results(results, cfg)
