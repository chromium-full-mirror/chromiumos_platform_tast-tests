# Copyright 2026 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import logging
import pathlib

from analyzer.backend import test_result
from analyzer.backend.perfetto_protos.protos.perfetto.trace_summary import (
    file_pb2,
)
from analyzer.backend.perfetto_protos.protos.perfetto.trace_summary import (
    v2_metric_pb2,
)
import click


MetricPolarity = v2_metric_pb2.TraceMetricV2Spec.MetricPolarity


def _load_results_from_trace_summary(
    summary: file_pb2.TraceSummary,
    run_id: str,
    test_name: str,
    label: str,
    unspecified_direction: test_result.ImprovementDirection | None,
) -> test_result.TestResults:
    out_results = test_result.TestResults()
    for bundle_i, bundle in enumerate(summary.metric_bundles):
        specs = bundle.specs
        # Specs match the values in each row. Precompute each spec's id, unit,
        # and improvement direction.
        spec_id = []
        spec_unit = []
        spec_direction = []
        for spec in specs:
            spec_id.append(spec.id)

            which_unit_oneof = spec.WhichOneof("unit_oneof")
            if which_unit_oneof == "unit":
                spec_unit.append(
                    v2_metric_pb2.TraceMetricV2Spec.MetricUnit.Name(spec.unit)
                )
            elif which_unit_oneof == "custom_unit":
                spec_unit.append(spec.custom_unit)
            else:
                spec_unit.append("unknown")

            polarity = spec.polarity
            if polarity == MetricPolarity.HIGHER_IS_BETTER:
                spec_direction.append(test_result.ImprovementDirection("up"))
            elif polarity == MetricPolarity.LOWER_IS_BETTER:
                spec_direction.append(test_result.ImprovementDirection("down"))
            elif polarity == MetricPolarity.POLARITY_UNSPECIFIED:
                # NB: If unspecified_direction we will skip these values.
                spec_direction.append(unspecified_direction)
            else:
                # Skip values with NOT_APPLICABLE or other polarity.
                spec_direction.append(None)

        for row_i, row in enumerate(bundle.row):
            metric_dimensions = []
            for dimension in row.dimension:
                dimension_field = dimension.WhichOneof("value_oneof")
                metric_dimensions.append(
                    str(getattr(dimension, dimension_field))
                )

            if len(row.values) > len(specs):
                logging.warning(
                    "Bundle %d row %d has more values (%d) than specs (%d) in %s %s",
                    bundle_i,
                    row_i,
                    len(row.values),
                    len(specs),
                    run_id,
                    test_name,
                )

            for value_i, value in enumerate(row.values):
                if value_i >= len(specs):
                    # We already warned about more values than specs, skip.
                    break
                if value.WhichOneof("value_oneof") == "null_value":
                    continue
                if spec_direction[value_i] is None:
                    continue

                metric_name = "-".join([spec_id[value_i], *metric_dimensions])
                variant = "summary"
                key = test_result.TestResultKey(
                    run_id=run_id,
                    test_name=test_name,
                    metric_name=metric_name,
                    variant=variant,
                    label=label,
                )
                out_results.results[key] = test_result.TestResult(
                    units=spec_unit[value_i],
                    improvement_direction=spec_direction[value_i],
                    value=value.double_value,
                )
    return out_results


def _load_results_from_web_tests_dir(
    path: pathlib.Path,
    label: str,
    unspecified_direction: test_result.ImprovementDirection | None,
) -> test_result.TestResults:
    # web-tests results directory structure is:
    # <run_timestamp>/<test_name>/pass/<run_timestamp>/runs/<iteration>
    paths = path.glob("*/*/pass/*/runs/*/trace_processor/v2_metrics.pb")

    all_results = test_result.TestResults()
    for path in paths:
        path_parts = path.parts
        run_timestamp = path_parts[-8]
        if run_timestamp == "latest":
            # Skip the "latest" symlink, it's pointing to one of the other dirs.
            continue
        run_iteration = path_parts[-3]
        test_name = path_parts[-7]
        run_id = run_timestamp + "_" + run_iteration
        summary = file_pb2.TraceSummary()
        summary.ParseFromString(path.read_bytes())
        results = _load_results_from_trace_summary(
            summary, run_id, test_name, label, unspecified_direction
        )
        all_results.merge(results)
    return all_results


@click.command()
@click.option(
    "--output-path",
    type=click.Path(
        exists=False, dir_okay=False, resolve_path=True, path_type=pathlib.Path
    ),
    default=pathlib.Path("data.json"),
    help="path to output summary JSON file",
)
@click.option(
    "--label",
    type=str,
    required=False,
    help="label for this experiment group - defaults to the file name of the output path",
)
@click.option(
    "--unspecified-direction",
    type=click.Choice(test_result.ImprovementDirection),
    required=False,
    help="What improvement direction to use for values with specs that have polarity POLARITY_UNSPECIFIED. Otherwise unspecified values will be skipped.",
)
@click.argument(
    "input-path",
    type=click.Path(
        exists=True, file_okay=False, resolve_path=True, path_type=pathlib.Path
    ),
)
def ingest_web_tests_results_directory(
    input_path: pathlib.Path,
    output_path: pathlib.Path,
    label: str | None,
    unspecified_direction: test_result.ImprovementDirection | None,
) -> None:
    """Ingest the web-tests results directory, like:
      web-tests/cuj/crossbench/runner/results.

    This will output a JSON file containing all the performance test results
    to `output_path'.

    Args:
        input_path: Path to the Tast results directory.
        output_path: Path to the output JSON file to create.
        label: optional label for this experiment group.
        unspecified_direction: optional improvement direction to use for metrics
            with POLARITY_UNSPECIFIED.
    """
    if label is None:
        label = output_path.name
    results = _load_results_from_web_tests_dir(
        input_path, label, unspecified_direction
    )
    output_path.write_text(results.to_json())
