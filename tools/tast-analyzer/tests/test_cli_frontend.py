# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import unittest

from analyzer.frontend import cli_frontend


class CliAnalysisTest(unittest.TestCase):
    def test_cli_analysis_no_changes(self) -> None:
        cli_frontend._compare_results(
            results=[],
            analyses=list(cli_frontend._CliAnalysis),
        )
