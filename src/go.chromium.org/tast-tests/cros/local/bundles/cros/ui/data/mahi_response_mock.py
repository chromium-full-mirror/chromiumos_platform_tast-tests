# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Mock http response."""

from mitmproxy import http


# This matches the fake response string in
# src/go.chromium.org/tast-tests/cros/local/ui/mahicuj/mahiutil/util.go
FAKE_RESPONSE_TEXT = "This is a fake response with text"


def request(flow: http.HTTPFlow) -> None:
    print("flow host is ", flow.request.host)
    if "aratea-pa.googleapis.com" in flow.request.host:
        response_bytes = (f"\n#\n!{FAKE_RESPONSE_TEXT}").encode("UTF-8")
        flow.response = http.Response.make(
            200,
            response_bytes,
            {"Content-Type": "application/x-protobuf"},
        )
