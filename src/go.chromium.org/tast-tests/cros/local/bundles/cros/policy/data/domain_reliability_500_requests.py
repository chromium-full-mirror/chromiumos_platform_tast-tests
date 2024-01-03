# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Inject 500 to HTTP response."""
from mitmproxy import http
from mitmproxy import dns
from mitmproxy import flow as f

def response(flow: http.HTTPFlow) -> None:
    if "google.com" in flow.request.host:
        flow.response = http.Response.make(
            500,
            b"Error injected by mitmproxy."
        )
