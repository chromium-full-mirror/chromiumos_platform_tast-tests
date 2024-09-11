# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import pathlib
import re
import xml.dom.minidom
import xml.etree.ElementTree as ET
from xml.sax import saxutils


class HtmlTree:
    html: ET.Element
    """The HTML element."""

    head: ET.Element
    """The <head> element in this HTML."""

    title: ET.Element
    """The <title> element in this HTML."""

    body: ET.Element
    """The <body> element in this HTML."""

    def __init__(self, path: pathlib.Path) -> None:
        if not path.exists():
            raise FileNotFoundError(f"{path} not found.")

        self.html = ET.fromstring(path.read_text())

        self.head = self._find_or_create_element("head", self.html)
        self.title = self._find_or_create_element("title", self.head)
        self.body = self._find_or_create_element("body", self.html)

    def _find_or_create_element(
        self, name: str, parent: ET.Element
    ) -> ET.Element:
        """Finds one and only one `<{name}>` element from the parent.

        If the element is not found, a new one is created and appended to the
        parent.

        Args:
            name: The tag name to find.
            parent: The parent to search.

        Returns:
            The found or created `<{name}>` element.
        """
        body_list = parent.findall(name)
        if len(body_list) == 0:
            element = ET.Element(name)
            parent.append(element)
        elif len(body_list) == 1:
            element = body_list[0]
        else:
            raise ValueError(f"Multiple <{name}> found")

        return element

    def __str__(self) -> str:
        dom = xml.dom.minidom.parseString(
            ET.tostring(self.html, encoding="utf-8")
        )

        pretty_xml = dom.toprettyxml()
        pretty_html = pretty_xml.replace('<?xml version="1.0" ?>', "")

        # `toprettyxml` escapes the text even within <style> tags.
        # We need to unescape it to make the style work.
        pretty_html = self._unescape_text_in_style(pretty_html)

        # `toprettyxml` appends a new line to the end of each line, which can
        # generate unnecessary blank lines. We check each line and filters it out
        # if it is empty.
        pretty_html = "\n".join(
            line for line in pretty_html.split("\n") if line.strip()
        )
        return pretty_html

    def _unescape_text_in_style(self, text: str) -> str:
        """Unescapes escaped text within <style> tags.

        Args:
            text: The input text.

        Returns:
            The text with escaped text unescaped.
        """

        def replace(match: re.Match) -> str:
            unescaped = saxutils.unescape(match.group(1), {r"&quot;": '"'})
            return f"<style>{unescaped}</style>"

        return re.sub(r"<style>(.*?)</style>", replace, text, flags=re.DOTALL)
