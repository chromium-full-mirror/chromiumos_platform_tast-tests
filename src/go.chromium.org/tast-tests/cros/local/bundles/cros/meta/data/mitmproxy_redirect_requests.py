# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Redirect HTTP requests to another server."""
from mitmproxy import http

def request(flow: http.HTTPFlow) -> None:
    if flow.request.host == "www.example.com":
        flow.request.host = "www.google.com"
