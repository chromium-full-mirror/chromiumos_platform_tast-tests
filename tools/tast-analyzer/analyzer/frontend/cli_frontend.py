# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.


from analyzer.analysis.analysis_results import AnalysisResult


def print_analysis_results(results: list[AnalysisResult]):
    """Prints a human readable summary of the analysis results."""
    for r in results:
        print(r.summary())
