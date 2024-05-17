# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
from analyzer.backend.tast_results_dir import ingest_tast_results_directory
import click


@click.group()
def cli() -> None:
    pass


cli.add_command(ingest_tast_results_directory, name="ingest-tast")

if __name__ == "__main__":
    cli()
