# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import pathlib

from analyzer.analysis import analysis_results
from analyzer.frontend.report import components
from analyzer.frontend.report import html_tree


class HtmlReport:
    results: list[analysis_results.AnalysisResult]
    """The results to make a report for."""

    template_dir: pathlib.Path
    """The path to the HTML template directory. """

    html: html_tree.HtmlTree
    """The HTML structure of the report."""

    def __init__(
        self,
        results: list[analysis_results.AnalysisResult],
        template_dir: pathlib.Path,
    ) -> None:
        self.results = results
        self.template_dir = template_dir
        self.html = html_tree.HtmlTree(template_dir / "index.html")
        self._set_title()

    def _test_names(self) -> list[str]:
        """Returns the test names to make a report for."""
        test_names: set[str] = set()
        for result in self.results:
            for pair in result.pairs:
                test_names |= set(pair.test_names())

        return sorted(test_names)

    def _set_title(self) -> None:
        """Sets the tile of the HTML from test names.

        The title is used in `<title>` and `<h1>` elements.
        """
        test_names = ", ".join(self._test_names())

        title = f"A/B Test Report | {test_names}"
        self.html.title.text = title

        heading = components.create_element_with_text("h1", title)
        self.html.body.append(heading)

    def write(self, output_dir: pathlib.Path) -> None:
        """Writes the report."""
        output_dir.mkdir(parents=True, exist_ok=True)
        (output_dir / "index.html").write_text(str(self.html))
