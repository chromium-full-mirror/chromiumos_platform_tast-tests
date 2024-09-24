# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import base64
import io
import xml.etree.ElementTree as ET

from analyzer.frontend import plot_util


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


def create_figure(
    plot_data: plot_util.PlotData,
    caption: str = "",
    attributes: dict[str, str] | None = None,
) -> ET.Element:
    """Creates a `<figure>` with `<img>` and `<figcaption>` inside.

    Args:
        plot_data: The PlotData from which to create `<figure>`.
        caption: The caption of the image. If not specified, `<figcaption>` is
            omitted from `<figure>`.
        attributes: A dictionary containing the `<figure>`'s attributes.
    """
    figure_element = ET.Element("figure", attributes or {})

    buffer = io.BytesIO()
    plot_data.figure.savefig(buffer, format="png", bbox_inches="tight")
    buffer.seek(0)

    prefix = "data"
    media_type = "image/png"
    token = "base64"
    data = base64.b64encode(buffer.read()).decode("utf-8")

    img = ET.Element(
        "img",
        {
            "src": f"{prefix}:{media_type};{token},{data}",
            "width": str(plot_data.width_px),
            "height": str(plot_data.height_px),
            "alt": caption,
        },
    )
    figure_element.append(img)

    if caption:
        figcatption = create_element_with_text("figcaption", caption)
        figure_element.append(figcatption)

    return figure_element
