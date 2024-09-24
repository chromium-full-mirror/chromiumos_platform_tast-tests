# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import pathlib
import unittest
import xml.etree.ElementTree as ET

from analyzer.analysis import analyze_results
from analyzer.analysis import metric_sample
from analyzer.backend import tast_results_dir


FILES_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute().joinpath("files")
)


def load_before_samples(
    test_name: str = "ui.OverviewPerf",
) -> list[metric_sample.MetricSample]:
    """Loads samples for the before group.

    Args:
        test_name: Test name of the samples.

    Returns:
        A list of loaded samples.
    """
    results = tast_results_dir._load_results_from_results_chart_json(
        path=pathlib.Path(f"/before/tests/{test_name}/results-chart.json"),
        json_str=FILES_DIR.joinpath("results-chart-analysis1.json").read_text(),
        label="before",
    )
    return analyze_results._load_samples_from_test_results(results)


def load_after_samples(
    test_name: str = "ui.OverviewPerf",
) -> list[metric_sample.MetricSample]:
    """Loads samples for the after group.

    Args:
        test_name: Test name of the samples.

    Returns:
        A list of loaded samples.
    """
    results = tast_results_dir._load_results_from_results_chart_json(
        path=pathlib.Path(f"/after/tests/{test_name}/results-chart.json"),
        json_str=FILES_DIR.joinpath("results-chart-analysis2.json").read_text(),
        label="after",
    )
    return analyze_results._load_samples_from_test_results(results)


def samples_by_id(
    samples: list[metric_sample.MetricSample],
) -> dict[str, metric_sample.MetricSample]:
    """Creates a map from sample id to sample.

    Args:
        samples: A list of samples to create a map for.

    Returns:
        A map from sample id to sample.
    """
    samples_by_id = {}
    for s in samples:
        assert s.sample_id not in samples_by_id
        samples_by_id[s.sample_id] = s
    return samples_by_id


def load_html(path: pathlib.Path) -> ET.Element:
    """Loads an HTML from the specified path.

    Args:
        path: The path from which the HTML is loaded.

    Returns:
        A loaded HTML parsed into ET.Element.
    """
    return ET.fromstring(path.read_text())


def _assert_html_text_equal(
    test: unittest.TestCase, text1: str | None, text2: str | None
) -> None:
    """Asserts if the text contents from two HTML elements are the same.

    When comparing, consecutive whitespace is normalized to a single space
    as in HTML.

    Args:
        test: The instance of unittest.TestCase to make an assertion for.
        text1: Text from an HTML element.
        text2: Text from another HTML element.
    """
    text1 = " ".join(text1.split()) if text1 else ""
    text2 = " ".join(text2.split()) if text2 else ""
    test.assertEqual(text1, text2)


def _assert_img_attributes_equal_except_data(
    test: unittest.TestCase,
    attributes1: dict[str, str],
    attributes2: dict[str, str],
) -> None:
    """Asserts if the attributes from two `<img>` elements are the same.

    If the `src` is a data URL, we only check the prefix:
    ("data:", mediatype, "base64"). This is because base64 encoded data may not
    be computed in a deterministic way even from the same input.

    Args:
        test: The instance of unittest.TestCase to make an assertion for.
        attributes1: Attributes from an `<img>` element.
        attributes2: Attributes from another `<img>` element.
    """
    src1 = attributes1.pop("src", "")
    src2 = attributes2.pop("src", "")
    test.assertDictEqual(attributes1, attributes2)

    data_url_prefix = "data:"

    if src1.startswith(data_url_prefix):
        test.assertTrue(src2.startswith(data_url_prefix))
        mediatype1, base64_token1 = (
            src1.removeprefix(data_url_prefix).split(",")[0].split(";")
        )
        mediatype2, base64_token2 = (
            src2.removeprefix(data_url_prefix).split(",")[0].split(";")
        )
        test.assertEqual(mediatype1, mediatype2)
        test.assertEqual(base64_token1, "base64")
        test.assertEqual(base64_token2, "base64")
    else:
        test.assertEqual(src1, src2)


def assert_elements_equal_except_image_data(
    test: unittest.TestCase, element1: ET.Element, element2: ET.Element
) -> None:
    """Asserts if two elements are the same.

    Base64 encoded data in `<img>` elements are excluded from the comparison.
    The assertion should be used if the elements may have `<img>` and there
    is no guarantee that their base64 encoded data is generated
    deterministically (e.g., when using a third-party library).

    Args:
        test: The instance of unittest.TestCase to make an assertion for.
        element1: An element.
        element2: Another element.
    """
    test.assertEqual(element1.tag, element2.tag)
    _assert_html_text_equal(test, element1.text, element2.text)
    if element1.tag == "img":
        _assert_img_attributes_equal_except_data(
            test, element1.attrib, element2.attrib
        )
    else:
        test.assertDictEqual(element1.attrib, element2.attrib)

    for child1, child2 in zip(element1, element2, strict=True):
        assert_elements_equal_except_image_data(test, child1, child2)
