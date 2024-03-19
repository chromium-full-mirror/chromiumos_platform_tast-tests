# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import logging

from mitmproxy import ctx
from mitmproxy.optmanager import load_paths


def load(loader):
    loader.add_option(
        name="allowed_endpoints_yaml",
        typespec=str | None,
        default=None,
        help="Enable yaml config for allow endpoints.",
    )


def running():
    if ctx.options.allowed_endpoints_yaml:
        logging.info(
            "Loading endpoints config: %s", ctx.options.allowed_endpoints_yaml
        )
        load_paths(ctx.options, ctx.options.allowed_endpoints_yaml)
