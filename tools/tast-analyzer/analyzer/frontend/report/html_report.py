# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from collections import defaultdict
import pathlib
import xml.etree.ElementTree as ET

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

    num_tables: int
    """The number of tables in the report."""

    def __init__(
        self,
        results: list[analysis_results.AnalysisResult],
        template_dir: pathlib.Path,
    ) -> None:
        self.results = results
        self.template_dir = template_dir
        self.html = html_tree.HtmlTree(template_dir / "index.html")
        self.num_tables = 0

    def _test_names(self) -> list[str]:
        """Returns the test names to make a report for."""
        test_names: set[str] = set()
        for result in self.results:
            for pair in result.pairs:
                test_names |= set(pair.test_names())

        return sorted(test_names)

    def _labels(self) -> list[str]:
        """Returns the labels used in the analysis."""
        labels: set[str] = set()
        for result in self.results:
            for pair in result.pairs:
                labels |= {pair.before.label(), pair.after.label()}

        return sorted(labels)

    def _metric_paths(self) -> list[str]:
        """Returns the metric paths used in the analysis."""
        metric_paths: set[str] = set()
        for result in self.results:
            for pair in result.pairs:
                metric_paths |= {
                    pair.before.metric_path(),
                    pair.after.metric_path(),
                }

        return sorted(metric_paths)

    def _create_sample_size_table(self) -> ET.Element:
        """Creates a table of sample sizes by label and metric path.

        This method generates an HTML table with a column for each label and
        a row for each metric. The table displays the sample size of each
        metric within each label.

        Returns:
            A `<table>` element.
        """
        metric_label_to_sample_size: defaultdict[
            str, dict[str, int]
        ] = defaultdict(lambda: defaultdict(int))

        for result in self.results:
            for group in result.groups:
                sample = group.sample
                metric_label_to_sample_size[sample.metric_path][
                    sample.label
                ] = sample.size()

        self.num_tables += 1

        caption = components.create_element_with_text(
            "caption", f"Table {self.num_tables}. Sample sizes."
        )

        thead = ET.Element("thead")
        thead_row = ET.SubElement(thead, "tr")
        thead_row.append(
            components.create_element_with_text("th", "Label", {"scope": "col"})
        )
        labels = self._labels()
        for label in labels:
            thead_row.append(
                components.create_element_with_text(
                    "th", label, {"data-is-numeric": "true", "scope": "col"}
                )
            )

        tbody = ET.Element("tbody")
        for metric_path in self._metric_paths():
            tr = ET.SubElement(tbody, "tr")
            tr.append(
                components.create_element_with_text(
                    "th", metric_path, {"scope": "row"}
                )
            )

            for label in labels:
                sample_size = metric_label_to_sample_size[metric_path].get(
                    label, "-"
                )
                tr.append(
                    components.create_element_with_text(
                        "td", f"{sample_size}", {"data-is-numeric": "true"}
                    )
                )

        table = ET.Element("table", {"id": f"table-{self.num_tables}"})
        table.append(caption)
        table.append(thead)
        table.append(tbody)

        return table

    def _set_title(self) -> None:
        """Sets the tile of the HTML from test names.

        The title is used in `<title>` and `<h1>` elements.
        """
        test_names = ", ".join(self._test_names())

        title = f"A/B Test Report | {test_names}"
        self.html.title.text = title

        heading = components.create_element_with_text("h1", title)
        self.html.body.append(heading)

    def make(self) -> None:
        """Makes a report."""

        self._set_title()

        table_container = ET.Element("div", {"class": "table-container"})
        table_container.append(self._create_sample_size_table())
        self.html.body.append(table_container)

    def write(self, output_dir: pathlib.Path) -> None:
        """Writes the report."""
        output_dir.mkdir(parents=True, exist_ok=True)

        styles = [path.read_text() for path in self.template_dir.glob("*.css")]
        style = components.create_element_with_text("style", "".join(styles))
        self.html.head.append(style)
        (output_dir / "index.html").write_text(str(self.html))
