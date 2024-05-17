# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
from pathlib import Path

import click


@click.command()
@click.argument(
    "input_path",
    type=click.Path(
        exists=True, file_okay=False, resolve_path=True, path_type=Path
    ),
)
def ingest_tast_results_directory(input_path: Path):
    """Ingest the Tast results directory, like: /tmp/tast/results/"""
    pass
