# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import dataclasses


@dataclasses.dataclass(frozen=True, kw_only=True, order=True)
class AnalysisCfg:
    skip_all_zero_samples: bool = True
    """Whether to skip all zero samples in the analysis."""

    minimum_sample_size: int = 1
    """The minimum sample size required to include a sample in the analysis."""
