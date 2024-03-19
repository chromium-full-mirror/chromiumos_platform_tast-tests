# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Inject 500 to HTTP response."""

from mitmproxy import http


def request(flow: http.HTTPFlow) -> None:
    if flow.request.host == "www.example.com":
        flow.response = http.Response.make(
            500, b"error injected by proxy", {"Content-Type": "text/html"}
        )
