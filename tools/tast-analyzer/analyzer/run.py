# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import logging

from analyzer.backend import tast_results_dir
from analyzer.backend import web_tests_results_dir
from analyzer.frontend import cli_frontend
import click


CONTEXT_SETTINGS = {
    "show_default": True,
}

import pathlib

from analyzer.backend.perfetto_protos.protos.perfetto.trace_summary import (
    file_pb2,
)
from google.protobuf import text_format


@click.command()
def convert_test_stuff():
    summary = file_pb2.TraceSummary()
    text_format.Parse(
        pathlib.Path("tests/files/v2_metrics.textproto").read_text(
            encoding="utf-8"
        ),
        summary,
    )
    with pathlib.Path("tests/files/v2_metrics.pb").open("wb") as f:
        f.write(summary.SerializeToString())


@click.group(context_settings=CONTEXT_SETTINGS)
def cli() -> None:
    pass


cli.add_command(convert_test_stuff, name="convert-test-stuff")
cli.add_command(
    tast_results_dir.ingest_tast_results_directory, name="ingest-tast"
)
cli.add_command(
    web_tests_results_dir.ingest_web_tests_results_directory,
    name="ingest-web-tests",
)
cli.add_command(cli_frontend.print_results, name="print-results")

if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    cli()
