// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

// dumpHTTPFlowAddon contains addon to dump HTTP flow.
const dumpHTTPFlowAddon = `import datetime
import json
import os

from flask import Flask
from flask import request as f_request
from mitmproxy import addonmanager
from mitmproxy import http
from mitmproxy.addons import asgiapp


traffic = {}


def load(loader: addonmanager.Loader):
    # Use set to remove duplicated URL.
    traffic["url"] = set()


def request(flow: http.HTTPFlow):
    traffic["url"].add(flow.request.url)


app = Flask("proxy_traffic_recorder")


# Please change testenv.DumpHTTPFlowURL if change the following line.
@app.route("/traffic")
def validate_allow_list() -> str:
    """
    Fetches traffic information and provides options for resetting the recorder and saving the data.

    Args:
        reset (bool, optional): If True, resets the traffic recorder. Defaults to True.
        outDir (str, optional): If provided, saves a JSON file containing traffic information
            to the specified directory.

    Returns:
        str: Traffic information formatted as a JSON string.
            Example: {"url": ["xxx"]}
    """
    reset = f_request.args.get("reset", "true", type=str).lower() == "true"
    outDir = f_request.args.get("outDir")

    json_string = json.dumps(traffic, cls=SetEncoder)

    if reset:
        traffic["url"].clear()

    if outDir:
        now = datetime.datetime.now()
        timestamp_str = now.strftime("%Y-%m-%d_%H-%M-%S")
        filename = f"traffic_{timestamp_str}.json"
        path = os.path.join(outDir, filename)
        with open(path, "a") as fp:
            fp.write(json_string)

    return json_string


addons = [
    # Please change testenv.DumpHTTPFlowURL if change the following line.
    asgiapp.WSGIApp(app, "proxy_server", 8080),
]


class SetEncoder(json.JSONEncoder):
    """
    Python is unable to convert set to json.
    Therefore, we need to change Set to List first.
    """

    def default(self, obj):
        if isinstance(obj, set):
            return list(obj)
        return json.JSONEncoder.default(self, obj)

`

const httpFlowFilterAddon = `from collections.abc import Sequence

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
    flow.response = http.Response.make(403)
    return
`
