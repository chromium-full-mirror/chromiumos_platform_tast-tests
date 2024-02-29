// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package histogram contains the files to creat histogram tabs such that similar memory resources
// will be used on tast tests as compared to windows tests
package histogram

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// HistogramTab holds the information of a Chrome browser tab.
type HistogramTab struct {
	histogram string
	conn      *chrome.Conn
}

// NewHistogramTab creates a new HistogramTab instance.
func NewHistogramTab(histogram string) *HistogramTab {
	return &HistogramTab{
		histogram: histogram,
		conn:      nil,
	}
}

// Close closes the HistogramTab connection.
func (tab *HistogramTab) Close(ctx context.Context) error {
	if tab.conn == nil {
		return nil
	}
	if err := tab.conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close tab")
	}
	if err := tab.conn.Close(); err != nil {
		return errors.Wrap(err, "failed to close tab connection")
	}
	tab.conn = nil
	return nil
}

type hist struct {
	Sample int     `json:"sample"`
	Mean   float64 `json:"mean"`
}

var histogramsWebArea = nodewith.Name("Histograms").Role(role.RootWebArea)

// CreateHistogramTabs creates histogram tabs to match windows test workload.
func CreateHistogramTabs(ctx context.Context, cr *chrome.Chrome, hist []string) (tabs []*HistogramTab, err error) {
	for _, h := range hist {
		tab := NewHistogramTab(h)
		tab.conn, err = cr.NewConn(ctx, "chrome://histograms/"+h)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open histogram")
		}
		tabs = append(tabs, tab)
	}
	return tabs, nil
}

// SwitchToMonitoringMode enables monitoring mode of chrome histogram tabs to match windows test workload.
func SwitchToMonitoringMode(ctx context.Context, ui *uiauto.Context, kb *input.KeyboardEventWriter, tabs []*HistogramTab) error {
	monitoringMode := nodewith.Name("Switch to Monitoring Mode").Role(role.Button)
	for idx, tab := range tabs {
		if err := tab.conn.ActivateTarget(ctx); err != nil {
			return errors.Wrap(err, "failed to activate target")
		}
		if err := uiauto.NamedCombine("switch to monitoring mode",
			ui.LeftClick(monitoringMode),
			ui.WaitUntilGone(monitoringMode),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to switch to tab index (%d)", idx+2)
		}
	}
	return nil
}

// ensureFileNotExist ensures that the file does not exist.
// If the file exists, remove it and return nil.
func ensureFileNotExist(filename string) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "failed to stat %s: %v", filename, err)
	}
	if err := os.Remove(filename); err != nil {
		return errors.Wrapf(err, "failed to remove %s file", filename)
	}
	return nil
}

// GenerateResults generates histogram result of chrome histogram tabs to match Windows tests work load.
func GenerateResults(ctx context.Context, ui *uiauto.Context, tabs []*HistogramTab, outDir string) error {
	dateStr := time.Now().Format("2006-01-02-15-04-05")
	resultFilename := fmt.Sprintf("%s_result.json", dateStr)
	if err := ensureFileNotExist(filepath.Join(outDir, resultFilename)); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(outDir, resultFilename))
	if err != nil {
		return errors.Wrapf(err, "failed to create %s file", resultFilename)
	}
	defer f.Close()

	histogramMap := make(map[string]hist)
	for _, tab := range tabs {
		if err := tab.conn.ActivateTarget(ctx); err != nil {
			return errors.Wrap(err, "failed to activate target")
		}

		sample, mean, err := readHistogram(ctx, ui)
		if err != nil {
			return errors.Wrap(err, "failed to get histogram")
		}
		histogramMap[tab.histogram] = hist{
			Sample: sample,
			Mean:   mean,
		}

		filename := fmt.Sprintf("%s_%s.png", tab.histogram, dateStr)
		imgPath := filepath.Join(outDir, filename)
		if err := ensureFileNotExist(imgPath); err != nil {
			return err
		}
		if err := screenshot.Capture(ctx, imgPath); err != nil {
			testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
		}
	}

	histogramJSON, err := json.Marshal(histogramMap)
	if err != nil {
		return errors.Wrap(err, "failed to marshal json")
	}
	if _, err := f.Write(histogramJSON); err != nil {
		return errors.Wrap(err, "failed to write to file")
	}
	return nil
}

// readHistogram gets histogram result of chrome histogram tabs to match Windows tests work load.
func readHistogram(ctx context.Context, ui *uiauto.Context) (sample int, mean float64, err error) {
	stopButton := nodewith.Name("Stop").Role(role.Button).Ancestor(histogramsWebArea)
	if err := ui.LeftClick(stopButton)(ctx); err != nil {
		return 0, 0, errors.Wrap(err, "failed to stop monitoring mode")
	}
	histogramHeader := nodewith.ClassName("histogram-header").Role(role.GenericContainer).Ancestor(histogramsWebArea)
	headers, err := ui.NodesInfo(ctx, histogramHeader)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to get histogram headers")
	}
	for id := range headers {
		histogramHeaderNode := histogramHeader.Nth(id)
		headerText := nodewith.NameStartingWith("Histogram:").Role(role.StaticText).Ancestor(histogramHeaderNode)
		header, err := ui.Info(ctx, headerText)
		if err != nil {
			return 0, 0, errors.Wrap(err, "failed to get header text")
		}
		testing.ContextLog(ctx, "header.Name: #", header.Name, "#", " ID: ", id)
		words := strings.Split(header.Name, " ")
		if len(words) < 8 {
			return 0, 0, errors.Wrap(err, "failed to get expected number of fields from header")
		}

		metricName := words[1]

		sample, err = strconv.Atoi(words[3])
		if err != nil {
			return 0, 0, errors.Wrap(err, "failed to convert sample")
		}
		mean, err = strconv.ParseFloat(words[7], 64)
		if err != nil {
			return 0, 0, errors.Wrap(err, "failed to convert mean")
		}
		testing.ContextLog(ctx, "metric: ", metricName, " samples: ", sample, " mean:", mean)

		expand := nodewith.Name("+").Role(role.StaticText).Ancestor(histogramHeaderNode)
		foundExpand, err := ui.IsNodeFound(ctx, expand)
		if err != nil {
			return 0, 0, errors.Wrap(err, "failed to find expand symbol")
		}
		if !foundExpand {
			continue
		}
		if err := ui.LeftClickUntil(expand, ui.WaitUntilGone(expand))(ctx); err != nil {
			return 0, 0, errors.Wrap(err, "failed to expand the details")
		}

		var histogramBodyFinder = nodewith.NameStartingWith("0").Role(role.StaticText).Ancestor(histogramsWebArea).Nth(id)
		if err := ui.WithTimeout(1 * time.Second).WaitUntilExists(histogramBodyFinder)(ctx); err != nil {
			return 0, 0, errors.Wrap(err, "failed to find histogramBody")
		}
		histogramBody := nodewith.NameStartingWith("0").Role(role.StaticText).Ancestor(histogramsWebArea).Nth(id)
		foundHistogramBody, err := ui.IsNodeFound(ctx, histogramBody)
		if err != nil {
			return 0, 0, errors.Wrap(err, "failed to find histogram body")
		}

		if !foundHistogramBody {
			testing.ContextLog(ctx, "ui.IsNodeFound(ctx, histogramBody) not found")
			continue
		}

		info, err := ui.Info(ctx, histogramBody)
		if err != nil {
			return 0, 0, errors.Wrap(err, "failed to get histogram body")
		}

		testing.ContextLog(ctx, "histogram body info.Name: ", fmt.Sprintf("\n%s", info.Name))
	}
	return sample, mean, nil
}
