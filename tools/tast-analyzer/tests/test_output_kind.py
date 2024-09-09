# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import unittest

from analyzer.frontend import output_kind
from analyzer.frontend import plot_util
from analyzer.frontend.report import report_util


class OutputKindTest(unittest.TestCase):
    def test_sort_output_kind(self) -> None:
        output_kinds: list[output_kind.OutputKind] = list(
            output_kind.OutputKind
        )
        result = output_kind.sort_output_kind(output_kinds)

        self.assertSetEqual(result[0], set(plot_util.PlotKind))
        self.assertSetEqual(result[1], set(report_util.ReportKind))
