# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Inject 500 to HTTP response."""

from mitmproxy import dns
from mitmproxy import flow as f
from mitmproxy import http


def response(flow: http.HTTPFlow) -> None:
    # images.google.com is blocked and access it to trigger domain reliability report.
    if "images.google.com" in flow.request.host:
        flow.response = http.Response.make(500, b"Error injected by mitmproxy.")
