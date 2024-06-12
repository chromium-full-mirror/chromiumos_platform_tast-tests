# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import unittest

from analyzer.analysis import stats_util


class StatsUtilTest(unittest.TestCase):
    def test_signed_change(self) -> None:
        self.assertEqual(stats_util.signed_change(1.0, 2.0), 1.0)
        self.assertEqual(stats_util.signed_change(2.0, 1.0), -0.5)
        self.assertEqual(stats_util.signed_change(1.0, 1.0), 0.0)
