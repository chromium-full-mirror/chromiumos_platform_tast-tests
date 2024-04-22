# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from collections.abc import Sequence

from mitmproxy import addonmanager
from mitmproxy import ctx
from mitmproxy import exceptions
from mitmproxy import http


def load(loader: addonmanager.Loader):
    loader.add_option(
        name="allowedHosts",
        typespec=Sequence[str],
        default=[],
        help="A list for allowed hostnames.",
    )


def configure(updated):
    """This method performs configuration validation,
    allowedHosts cannot be empty.
    """
    allowedHosts = ctx.options.allowedHosts

    if not allowedHosts:
        raise exceptions.OptionsError("allowedHosts should be non-empty")


def request(flow: http.HTTPFlow) -> None:
    """This method determines whether to allow traffic based on
    allowedHosts.

    1. Only traffic in allowedHosts is allowed.
    2. Otherwise, the traffic is blocked and return 503.
    """
    allowedHosts = ctx.options.allowedHosts
    hostname = flow.request.host

    # Case 1: allowedURLs is present.
    # We only allow URLs in allowedURLs and block anything else.
    for allowed in allowedHosts:
        if hostname == allowed:
            return
    # The HTTP 403 Forbidden response status code indicates that the server
    # understands the request but refuses to authorize it.
    # Use 403 to indicates that the request is blocked by MitmProxy.
    flow.response = http.Response.make(403)
    return
