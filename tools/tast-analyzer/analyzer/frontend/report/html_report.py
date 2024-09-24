# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from collections import defaultdict
import pathlib
import xml.etree.ElementTree as ET

from analyzer.analysis import analysis_cfg
from analyzer.analysis import analysis_results
from analyzer.frontend import plot_util
from analyzer.frontend.report import components
from analyzer.frontend.report import html_tree


class HtmlReport:
    results: list[analysis_results.AnalysisResult]
    """The results to make a report for."""

    template_dir: pathlib.Path
    """The path to the HTML template directory. """

    cfg: analysis_cfg.AnalysisCfg
    """The configuration for the statistical analysis."""

    identifier_to_plots_map: dict[str, list[plot_util.PlotData]]
    """The mapping from pairwise result identifiers to lists of their plot data."""

    html: html_tree.HtmlTree
    """The HTML structure of the report."""

    num_tables: int
    """The number of tables in the report."""

    num_figures: int
    """The number of figures in the report."""

    def __init__(
        self,
        results: list[analysis_results.AnalysisResult],
        template_dir: pathlib.Path,
        cfg: analysis_cfg.AnalysisCfg,
        identifier_to_plots_map: dict[str, list[plot_util.PlotData]],
    ) -> None:
        self.results = results
        self.template_dir = template_dir
        self.cfg = cfg
        self.identifier_to_plots_map = identifier_to_plots_map
        self.html = html_tree.HtmlTree(template_dir / "index.html")
        self.num_tables = 0
        self.num_figures = 0

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

        pair_identifiers = sorted(
            [
                pair.identifier()
                for result in self.results
                for pair in result.pairs
            ]
        )

        ul = ET.SubElement(self.html.body, "ul")
        for identifier in pair_identifiers:
            ul.append(components.create_element_with_text("li", identifier))

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
        plot_data: plot_util.PlotData,
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
        self, pair: analysis_results.PairwiseResult
    ) -> None:
        """Appends a summary of the given pairwise result to the HTML.

        Args:
            pair: The pairwise result to make a summary for.
        """

        self.html.body.append(
            components.create_element_with_text("h2", pair.identifier())
        )
        self.html.body.append(self._create_pairwise_result_table(pair))

        for plot in self.identifier_to_plots_map[pair.identifier()]:
            self.html.body.append(
                self._create_pairwise_result_figure(pair, plot)
            )

    def make(self) -> None:
        """Makes a report."""

        self._set_title()

        self._append_summary()
        table_container = ET.Element("div", {"class": "table-container"})
        table_container.append(self._create_sample_size_table())
        self.html.body.append(table_container)

        for result in self.results:
            for pair in result.pairs:
                self._append_pairwise_result_summary(pair)

    def write(self, output_dir: pathlib.Path) -> None:
        """Writes the report."""
        output_dir.mkdir(parents=True, exist_ok=True)

        styles = [path.read_text() for path in self.template_dir.glob("*.css")]
        style = components.create_element_with_text("style", "".join(styles))
        self.html.head.append(style)
        (output_dir / "index.html").write_text(str(self.html))
