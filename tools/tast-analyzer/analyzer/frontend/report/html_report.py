# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import base64
from collections import defaultdict
import gzip
import json
import pathlib
import xml.etree.ElementTree as ET

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.backend import test_result
from analyzer.frontend import output
from analyzer.frontend import plot
from analyzer.frontend import plot_util
from analyzer.frontend.report import components
from analyzer.frontend.report import html_tree


class HtmlReport:
    raw_test_results: list[test_result.TestResults]
    """The raw data before statistical processing."""

    results: list[output.AnalysisResultForOutput]
    """The results to make a report for."""

    template_dir: pathlib.Path
    """The path to the HTML template directory. """

    cfg: analysis_cfg.AnalysisCfg
    """The configuration for the statistical analysis."""

    html: html_tree.HtmlTree
    """The HTML structure of the report."""

    num_tables: int
    """The number of tables in the report."""

    num_figures: int
    """The number of figures in the report."""

    def __init__(
        self,
        raw_test_results: list[test_result.TestResults],
        results: list[output.AnalysisResultForOutput],
        template_dir: pathlib.Path,
        cfg: analysis_cfg.AnalysisCfg,
    ) -> None:
        self.raw_test_results = raw_test_results
        self.results = results
        self.template_dir = template_dir
        self.cfg = cfg
        self.html = html_tree.HtmlTree(template_dir / "index.html")
        self.num_tables = 0
        self.num_figures = 0

    def _test_names(self) -> list[str]:
        """Returns the test names to make a report for."""
        test_names: set[str] = set()
        for result in self.results:
            for pair in result.pairs:
                test_names |= set(pair.result.test_names())

        return sorted(test_names)

    def _labels(self) -> list[str]:
        """Returns the labels used in the analysis."""
        labels: set[str] = set()
        for result in self.results:
            for pair in result.pairs:
                labels |= {
                    pair.result.before.label(),
                    pair.result.after.label(),
                }

        return sorted(labels)

    def _metric_paths(self) -> list[str]:
        """Returns the metric paths used in the analysis."""
        metric_paths: set[str] = set()
        for result in self.results:
            for pair in result.pairs:
                metric_paths |= {
                    pair.result.before.metric_path(),
                    pair.result.after.metric_path(),
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

    def _append_summary(self) -> None:
        """Appends a summary of all analysis results to the HTML."""

        h2 = components.create_element_with_text("h2", "Summary")
        self.html.body.append(h2)

        p = ET.SubElement(self.html.body, "p")
        if not self.results:
            p.text = "No statistically significant differences were detected."
            return
        p.text = (
            "Statistically significant differences were detected "
            "in the following pairs:"
        )

        ul = ET.SubElement(self.html.body, "ul")
        for result in self.results:
            groups_name = plot_util.get_groups_name_for_plot(result.groups)
            li = ET.SubElement(ul, "li")
            li.append(
                components.create_element_with_text(
                    "a", groups_name, {"href": f"#{groups_name}"}
                )
            )
            inner_ul = ET.SubElement(ul, "ul")
            for pair in sorted(result.pairs):
                inner_li = ET.SubElement(inner_ul, "li")
                pair_id = pair.result.identifier()
                inner_li.append(
                    components.create_element_with_text(
                        "a", pair_id, {"href": f"#{pair_id}"}
                    )
                )

    def _create_pairwise_result_table(
        self, pair: analysis_results.PairwiseResult
    ) -> ET.Element:
        """Creates a summary table for the given pairwise result.

        Args:
            pair: The pairwise result to make a summary table for.

        Returns:
            A `<table>` element.
        """
        metric_names = pair.metric_names()

        self.num_tables += 1
        caption = components.create_element_with_text(
            "caption",
            f"Table {self.num_tables}. Change in the mean of "
            f"{'/'.join(metric_names)} in {pair.units()}. "
            f"{'Higher' if pair.is_up_better() else 'Lower'} is better.",
        )

        thead = ET.Element("thead")
        thead_row = ET.Element("tr")
        confidence = self.cfg.bootstrap_params.confidence
        headers = (
            ["Label"]
            # Show metrics only if they are different
            + (["Metric"] if len(metric_names) > 1 else [])
            + ["Mean ± std", "ΔMean", f"{confidence:.0%} CI"]
        )
        for header in headers:
            th = components.create_element_with_text(
                "th", header, {"scope": "col"}
            )
            if header != "Label":
                th.set("data-is-numeric", "true")
            thead_row.append(th)
        thead.append(thead_row)

        tbody = ET.Element("tbody")
        before_row = ET.Element("tr")
        after_row = ET.Element("tr")
        for row, group in zip(
            [before_row, after_row], [pair.before, pair.after]
        ):
            row.append(
                components.create_element_with_text(
                    "th", group.label(), {"scope": "row"}
                )
            )
            if len(metric_names) > 1:
                row.append(
                    components.create_element_with_text(
                        "td", group.metric_name(), {"data-is-numeric": "true"}
                    )
                )
            row.append(
                components.create_element_with_text(
                    "td",
                    f"{group.sample.mean():.2f} ± {group.sample.std():.2f}",
                    {"data-is-numeric": "true"},
                )
            )

            mean_change_cell = ET.Element("td", {"data-is-numeric": "true"})
            if row == before_row:
                mean_change_cell.text = "-"
            else:
                proportion_change = pair.mean_change_better()
                mean_change_cell.text = f"{proportion_change:.2%}"
                mean_change_cell.set(
                    "class", "better" if proportion_change > 0 else "worse"
                )
            row.append(mean_change_cell)

            ci_cell = ET.Element("td", {"data-is-numeric": "true"})
            if (bootstrap := group.bootstrap) is None:
                ci_cell.text = "-"
            else:
                low = bootstrap.confidence_interval.low
                high = bootstrap.confidence_interval.high
                ci_cell.text = f"[{low:.3f}, {high:.3f}]"
            row.append(ci_cell)
        tbody.append(before_row)
        tbody.append(after_row)

        table = ET.Element("table", {"id": f"table-{self.num_tables}"})
        table.append(caption)
        table.append(thead)
        table.append(tbody)

        return table

    def _create_pairwise_result_figure(
        self,
        pair: analysis_results.PairwiseResult,
        plot_data: plot.PlotData,
    ) -> ET.Element:
        """Creates a `<figure>` element from the given plot data for the given
        pairwise result.

        Args:
            pair: The pairwise result to make a `<figure>` element for.
            plot_data: The plot data used to create the `<figure>` element.

        Returns:
            A `<figure>` element.
        """
        self.num_figures += 1

        return components.create_figure(
            plot_data=plot_data,
            caption=f"Figure {self.num_figures}. "
            f"{plot_data.kind.description()} of {'/'.join(pair.metric_names())}.",
            attributes={"id": f"figure-{self.num_figures}"},
        )

    def _append_pairwise_result_summary(
        self, pair: output.PairwiseResultForOutput
    ) -> None:
        """Appends a summary of the given pairwise result to the HTML.

        Args:
            pair: The pairwise result to make a summary for.
        """

        pair_id = pair.result.identifier()
        self.html.body.append(
            components.create_element_with_text("h3", pair_id, {"id": pair_id})
        )
        self.html.body.append(self._create_pairwise_result_table(pair.result))

        for plot_data in pair.plots:
            self.html.body.append(
                self._create_pairwise_result_figure(pair.result, plot_data)
            )

    def _append_groups_result_figure(
        self,
        groups_id: str,
        plot_data: plot.PlotData,
    ) -> None:
        """Appends a `<figure>` element of the groups level plot to the HTML body.

        Args:
            groups_id: The groups ID of the plot.
            plot_data: The plot data used to create the `<figure>` element.
        """
        self.num_figures += 1
        self.html.body.append(
            components.create_figure(
                plot_data=plot_data,
                caption=f"Figure {self.num_figures}. "
                f"{plot_data.kind.description()} of {groups_id}.",
                attributes={"id": f"figure-{self.num_figures}"},
            )
        )

    def _append_groups_summary(
        self, result: output.AnalysisResultForOutput
    ) -> None:
        """Appends a summary of the given analysis result of certain groups
        to the HTML.

        Args:
            result: The groups level analysis result to make a summary for.
        """
        groups_name = plot_util.get_groups_name_for_plot(result.groups)
        self.html.body.append(
            components.create_element_with_text(
                "h2", groups_name, {"id": groups_name}
            )
        )

        # If the number of samples is two, groups level figures are the same
        # as pairwise result figures. To avoid duplication, we show groups
        # level figures only if there are more than two samples.
        if len(result.groups) > 2:
            for result_plot in result.groups_plots:
                self._append_groups_result_figure(groups_name, result_plot)

        for pair in result.pairs:
            self._append_pairwise_result_summary(pair)

    def _embed_raw_data(self) -> None:
        """Embeds raw data in the report."""
        script = ET.SubElement(
            self.html.html,
            "script",
            {"id": "raw-data", "type": "application/octet-stream"},
        )
        raw_data_str = json.dumps(
            [data.to_json() for data in self.raw_test_results]
        )
        compressed = gzip.compress(raw_data_str.encode())
        script.text = base64.b64encode(compressed).decode()

    def make(self) -> None:
        """Makes a report."""

        self._set_title()

        self._append_summary()
        table_container = ET.Element("div", {"class": "table-container"})
        table_container.append(self._create_sample_size_table())
        self.html.body.append(table_container)

        for result in self.results:
            self._append_groups_summary(result=result)

        self._embed_raw_data()

    def write(self, output_dir: pathlib.Path) -> None:
        """Writes the report."""
        output_dir.mkdir(parents=True, exist_ok=True)

        styles = [path.read_text() for path in self.template_dir.glob("*.css")]
        style = components.create_element_with_text("style", "".join(styles))
        self.html.head.append(style)
        (output_dir / "index.html").write_text(str(self.html))
