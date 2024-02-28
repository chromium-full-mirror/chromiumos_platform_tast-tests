# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from collections.abc import Sequence
import os
import re

from mitmproxy import ctx
from mitmproxy import http


class TrafficLogger:
    def load(self, loader):
        loader.add_option(
            name="endpoint_info_folder",
            typespec=str,
            default="",
            help="Specify the folder to store endpoint info",
        )

        loader.add_option(
            name="patterns_to_record",
            typespec=Sequence[str],
            default=[],
            help="Specify the the patterns of traffic to record. If it's empty, then we record everything. Otherwise, we record traffic solely when the URL matches any regex pattern.",
        )

    def request(self, flow: http.HTTPFlow):
        folder = ctx.options.endpoint_info_folder
        patterns_to_record = ctx.options.patterns_to_record

        save_to_logfile = False
        for pattern in patterns_to_record:
            if re.search(pattern, flow.request.url):
                save_to_logfile = True
                break
        else:
            save_to_logfile = not patterns_to_record

        if save_to_logfile:
            path = os.path.join(folder, "endpoints.log")
            with open(path, "a") as fp:
                fp.write(f"{flow.request.host} {flow.request.url}\n")


addons = [TrafficLogger()]
