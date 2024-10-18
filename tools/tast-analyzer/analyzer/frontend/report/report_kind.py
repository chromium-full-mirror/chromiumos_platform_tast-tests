# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import enum


class ReportKind(enum.StrEnum):
    """Kind of report to create."""

    REPORT_HTML = "report-html"
    """Creates an HTML report."""
