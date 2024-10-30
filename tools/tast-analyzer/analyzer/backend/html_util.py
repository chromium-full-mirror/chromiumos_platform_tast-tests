# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import re
from xml.sax import saxutils


def escape_text_in_style(text: str) -> str:
    """Escapes special characters within <style> tags.

    Args:
        text: The input text.

    Returns:
        The text with special characters escaped.
    """

    def replace(match: re.Match) -> str:
        escaped = saxutils.escape(match.group(1))
        return f"<style>{escaped}</style>"

    return re.sub(r"<style>(.*?)</style>", replace, text, flags=re.DOTALL)
