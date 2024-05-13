# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Mock http response."""

from mitmproxy import http


def request(flow: http.HTTPFlow) -> None:
    if flow.request.host == "www.example.com":
        flow.response = http.Response.make(
            200, b"Content injected by proxy", {"Content-Type": "text/html"}
        )
