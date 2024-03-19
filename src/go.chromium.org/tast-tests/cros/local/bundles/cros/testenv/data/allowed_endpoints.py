# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from collections.abc import Sequence
import logging

from mitmproxy import ctx
from mitmproxy import http
from mitmproxy.optmanager import load_paths


def load(loader):
    loader.add_option(
        name="allowed_endpoints",
        typespec=Sequence[str],
        default=[],
        help="Endpoints to allow.",
    )


def request(flow: http.HTTPFlow) -> None:
    logging.info(ctx.options.allowed_endpoints)
    logging.info(flow.request.host)
    if flow.request.host not in ctx.options.allowed_endpoints:
        flow.response = http.Response.make(500, b"disabled in tests")
