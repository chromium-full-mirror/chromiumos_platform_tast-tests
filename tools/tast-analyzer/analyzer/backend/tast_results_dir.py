# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import json
import pathlib

from analyzer.backend import test_result
import click


def _load_results_from_results_chart_json(
    path: pathlib.Path, json_str: str
) -> test_result.TestResults:
    out_results = test_result.TestResults()
    # Tast results dir format is: <test run id>/tests/<test name>/
    test_run_id = path.parts[-4]
    test_name = path.parts[-2]
    for metric_name, in_results in json.loads(json_str).items():
        for variant, in_result in in_results.items():
            key = test_result.TestResultKey(
                run_id=test_run_id,
                test_name=test_name,
                metric_name=metric_name,
                variant=variant,
            )
            assert key not in out_results.results, f"duplicate key: {key}"

            kind = in_result["type"]
            value: int | float | list[float]
            if kind == "scalar":
                assert isinstance(
                    in_result["value"], float | int
                ), f"expected int/float, got {in_result['value']}"
                value = in_result["value"]
            elif kind == "list_of_scalar_values":
                assert isinstance(
                    in_result["values"], list
                ), f"expected list, got {in_result['values']}"
                value = in_result["values"]
            else:
                assert False, f"unknown type: {kind}"

            out_results.results[key] = test_result.TestResult(
                units=in_result["units"],
                improvement_direction=test_result.ImprovementDirection(
                    in_result["improvement_direction"]
                ),
                value=value,
            )
    return out_results


def _load_results_from_tast_dir(
    path: pathlib.Path,
) -> test_result.TestResults:
    """Extracts values from results-chart.json and returns them as a dictionary."""

    paths = path.glob("*/tests/*/results-chart.json")
    all_results = test_result.TestResults()
    for path in paths:
        results = _load_results_from_results_chart_json(path, path.read_text())
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
@click.argument(
    "input-path",
    type=click.Path(
        exists=True, file_okay=False, resolve_path=True, path_type=pathlib.Path
    ),
)
def ingest_tast_results_directory(
    input_path: pathlib.Path, output_path: pathlib.Path
) -> None:
    """Ingest the Tast results directory, like: /tmp/tast/results/.

    This will output a JSON file containing all the performance test results
    to `output_path'.

    Args:
        input_path: Path to the Tast results directory.
        output_path: Path to the output JSON file to create.
    """
    results = _load_results_from_tast_dir(input_path)
    output_path.write_text(results.to_json())
