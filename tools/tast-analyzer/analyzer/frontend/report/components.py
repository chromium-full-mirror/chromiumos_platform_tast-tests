# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import xml.etree.ElementTree as ET


def create_element_with_text(
    tag: str, text: str, attributes: dict[str, str] = {}
) -> ET.Element:
    """Creates an `ET.Element` with the given text set.

    Args:
        tag: A string identifying the XML tag name of this element.
        text: Text to go in this element.
        attrib: A dictionary containing the element's attributes.

    Returns:
        An XML element with the text and the attributes set.
    """
    element = ET.Element(tag, attributes)
    element.text = text
    return element
