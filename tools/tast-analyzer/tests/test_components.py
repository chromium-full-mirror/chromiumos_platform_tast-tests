# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import unittest

from analyzer.frontend.report import components


class ComponentsTest(unittest.TestCase):
    def test_create_element_with_text(self) -> None:
        tag = "p"
        text = "placeholder"
        attributes = {"id": "placeholder"}
        component = components.create_element_with_text(tag, text, attributes)
        self.assertEqual(component.tag, tag)
        self.assertEqual(component.text, text)
        self.assertEqual(component.attrib, attributes)
