# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import pathlib
import unittest

from analyzer.frontend import plot_util
from analyzer.frontend.report import components
from matplotlib import figure
from tests import test_util


COMPONENTS_DIR: pathlib.Path = pathlib.Path(
    pathlib.Path(__file__).parent.absolute() / "files" / "report" / "components"
)


class ComponentsTest(unittest.TestCase):
    def test_create_element_with_text(self) -> None:
        tag = "p"
        text = "placeholder"
        attributes = {"id": "placeholder"}
        component = components.create_element_with_text(tag, text, attributes)
        self.assertEqual(component.tag, tag)
        self.assertEqual(component.text, text)
        self.assertEqual(component.attrib, attributes)

    def test_create_figure_with_caption(self) -> None:
        figure_element = components.create_figure(
            plot_data=plot_util.PlotData(
                kind=plot_util.PlotKind.PLOT_BOX, figure=figure.Figure()
            ),
            caption="placeholder",
            attributes={"class": "placeholder"},
        )
        expected_figure = test_util.load_html(
            COMPONENTS_DIR / "figure_with_caption.html"
        )
        test_util.assert_elements_equal_except_image_data(
            self, figure_element, expected_figure
        )

    def test_create_figure_without_caption(self) -> None:
        figure_element = components.create_figure(
            plot_data=plot_util.PlotData(
                kind=plot_util.PlotKind.PLOT_BOX,
                figure=figure.Figure(),
            ),
            attributes={"class": "placeholder"},
        )
        expected_figure = test_util.load_html(
            COMPONENTS_DIR / "figure_without_caption.html"
        )
        test_util.assert_elements_equal_except_image_data(
            self, figure_element, expected_figure
        )
