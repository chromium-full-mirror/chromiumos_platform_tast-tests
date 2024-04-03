# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import datetime
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


# Please change URL in mitmproxy.DumpHTTPFlow if change the following line.
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
    # Please change URL in mitmproxy.DumpHTTPFlow if change the following line.
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
