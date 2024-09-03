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


def _load_samples_from_test_results(
    results: test_result.TestResults,
) -> list[metric_sample.MetricSample]:
    samples_by_id: dict[str, metric_sample.MetricSample] = {}

    logging.info(f"Examining {len(results.results)} records")
    for key, result in sorted(results.results.items()):
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

        s = samples_by_id.setdefault(
            key.sample_id(),
            metric_sample.MetricSample(
                label=key.label,
                sample_id=key.sample_id(),
                test_name=key.test_name,
                metric_name=key.metric_name,
                metric_path=key.metric_path(),
                units=result.units,
                improvement_direction=result.improvement_direction,
                value_map={},
            ),
        )

        assert s.label == key.label
        assert s.sample_id == key.sample_id()
        assert s.test_name == key.test_name
        assert s.metric_path == key.metric_path()
        assert s.units == result.units
        assert s.improvement_direction == result.improvement_direction
        assert key.run_id not in s.value_map
        s.value_map[key.run_id] = val

    logging.info(f"Loaded {len(samples_by_id)} samples")
    sample_sizes: dict[int, int] = defaultdict(int)
    for v in samples_by_id.values():
        sample_size = len(v.value_map)
        sample_sizes[sample_size] += 1
    logging.info(f"Sample size distribution: {sorted(sample_sizes.items())}")

    return list(samples_by_id.values())


def _load_samples_from_paths(
    paths: list[pathlib.Path],
) -> list[metric_sample.MetricSample]:
    all_samples: list[metric_sample.MetricSample] = []
    sample_ids: set[str] = set()
    for path in paths:
        results = test_result.TestResults.from_json(path.read_text())

        # Set labels to the filename if the results version is old.
        if results.metadata.results_version < 2:
            migrated_results = {}
            for result_key in list(results.results.keys()):
                assert result_key.label == ""
                migrated_key = dataclasses.replace(result_key, label=path.name)
                migrated_results[migrated_key] = results.results[result_key]
                del results.results[result_key]
            results.results.update(migrated_results)

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

    out_results = []
    p_values = [r.hypothesis_result.p for r in results]
    rejects, p_corrected, _, _ = multitest.multipletests(
        p_values, alpha=cfg.alpha, method=cfg.multiple_test_cfg.scipy_name()
    )
    for r, reject, p in zip(results, rejects, p_corrected):
        # Reject the null hypothesis (that they are the same).
        if reject:
            r = copy.deepcopy(r)
            r.hypothesis_result.p = p
            out_results.append(r)
    return out_results


def _prune_regex_include(
    samples: list[metric_sample.MetricSample], regex: str
) -> list[metric_sample.MetricSample]:
    return [s for s in samples if re.search(regex, s.metric_path)]


def _prune_regex_exclude(
    samples: list[metric_sample.MetricSample], regex: str
) -> list[metric_sample.MetricSample]:
    return [s for s in samples if not re.search(regex, s.metric_path)]


def _prune_persistent_cfg(
    samples: list[metric_sample.MetricSample], cfg: analysis_cfg.PersistentCfg
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
        vals = sorted(s.value_map.items(), key=lambda x: x[1])
        if len(vals):
            del vals[0]
        if len(vals):
            del vals[-1]
        out_samples.append(dataclasses.replace(s, value_map=dict(vals)))
    return out_samples


def _prune_all_zero_samples(
    samples: list[metric_sample.MetricSample],
) -> list[metric_sample.MetricSample]:
    out_samples = []
    for s in samples:
        if any(i != 0.0 for i in s.value_map.values()):
            out_samples.append(s)
    return out_samples


def _prune_minimum_sample_size(
    samples: list[metric_sample.MetricSample],
    sample_size: int,
) -> list[metric_sample.MetricSample]:
    return [s for s in samples if len(s.value_map) >= sample_size]


def analyze_results(
    sample1_path: pathlib.Path,
    sample2_path: pathlib.Path,
    cfg: analysis_cfg.AnalysisCfg,
) -> list[analysis_results.AnalysisResult]:
    """Returns AnalysisResults for the given saved sample data paths."""
    before_samples = _load_samples_from_paths([sample1_path])
    after_samples = _load_samples_from_paths([sample2_path])

    if cfg.remove_outliers:
        before_samples = _prune_outliers(before_samples)
        after_samples = _prune_outliers(after_samples)

    before_samples = _prune_persistent_cfg(before_samples, cfg.persistent_cfg)
    after_samples = _prune_persistent_cfg(after_samples, cfg.persistent_cfg)

    if cfg.metric_include_regex:
        before_samples = _prune_regex_include(
            before_samples, cfg.metric_include_regex
        )
        after_samples = _prune_regex_include(
            after_samples, cfg.metric_include_regex
        )

    if cfg.metric_exclude_regex:
        before_samples = _prune_regex_exclude(
            before_samples, cfg.metric_exclude_regex
        )
        after_samples = _prune_regex_exclude(
            after_samples, cfg.metric_exclude_regex
        )

    # Skip any things with just zeros - seems to happen for broken tests.
    if cfg.skip_all_zero_samples:
        before_samples = _prune_all_zero_samples(before_samples)
        after_samples = _prune_all_zero_samples(after_samples)

    before_samples = _prune_minimum_sample_size(
        before_samples, cfg.minimum_sample_size
    )
    after_samples = _prune_minimum_sample_size(
        after_samples, cfg.minimum_sample_size
    )

    metric_paths = analysis_results.compute_metric_paths_for_comparison(
        s1=before_samples, s2=after_samples
    )
    results = analysis_results.generate_analysis_results(
        before_samples=before_samples,
        after_samples=after_samples,
        metric_paths=metric_paths,
        hypothesis_params=cfg.hypothesis_test_params,
        bootstrap_params=cfg.bootstrap_params,
    )

    return _prune_non_significant_results(results, cfg)
