# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import unittest

from analyzer.frontend import output
from analyzer.frontend import plot
from analyzer.frontend.report import report_kind


class OutputKindTest(unittest.TestCase):
    def test_sort_output(self) -> None:
        outputs: list[output.OutputKind] = list(output.OutputKind)
        plot_kinds, report_kinds = output.sort_output_kind(outputs)

        self.assertSetEqual(plot_kinds, set(plot.PlotKind))
        self.assertSetEqual(report_kinds, set(report_kind.ReportKind))
