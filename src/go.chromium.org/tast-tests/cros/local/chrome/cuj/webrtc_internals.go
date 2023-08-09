// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/webrtcinternals"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const createDumpSectionName = "Create Dump"

var (
	createDumpSectionReg = regexp.MustCompile("(Create Dump)|(Create a WebRTC-Internals dump)")
	createDumpSection    = nodewith.NameRegex(createDumpSectionReg).Role(role.DisclosureTriangle)
	webRTCRootWebArea    = nodewith.Name("WebRTC Internals").Role(role.RootWebArea)
	webRTCDownloadButton = nodewith.NameContaining("Download").Role(role.Button).Ancestor(webRTCRootWebArea)
)

// ExpandCreateDumpSection expands the Create Dump section of chrome://webrtc-internals.
// We will not need it until after the meeting, but we can expand the section much faster now
// while chrome://webrtc-internals does not have much data to show.
func ExpandCreateDumpSection(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	return uiauto.NamedCombine(fmt.Sprintf("expand %q section", createDumpSectionName),
		ui.WaitUntilExists(createDumpSection.Collapsed()),
		ui.DoDefaultUntil(createDumpSection, ui.WithTimeout(5*time.Second).WaitUntilExists(createDumpSection.Expanded())),
		ui.WaitUntilExists(webRTCDownloadButton),
	)(ctx)
}

// OpenWebRTCInternals opens chrome://webrtc-internals now so it will collect data on the meeting's streams.
func OpenWebRTCInternals(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser) (*browser.Conn, error) {
	conn, err := br.NewTab(ctx, WebRTCInternalsURL, browser.WithNewWindow())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open %s", WebRTCInternalsURL)
	}

	if err := ExpandCreateDumpSection(ctx, tconn); err != nil {
		return nil, errors.Wrapf(err, "failed to expand %q section in %s", createDumpSectionName, WebRTCInternalsURL)
	}

	return conn, nil
}

// DumpWebRTCInternals downloads a dump from chrome://webrtc-internals and
// returns the file path. This function assumes that chrome://webrtc-internals
// is already shown, with the Create Dump section expanded.
func DumpWebRTCInternals(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, username string) (dumpFilePath string, err error) {
	downloadsPath, err := cryptohome.DownloadsPath(ctx, username)
	if err != nil {
		return "", errors.Wrap(err, "failed to get Downloads path")
	}

	out, err := testexec.CommandContext(ctx, "ls", "-l", downloadsPath).Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to list Downloads directory")
	}
	testing.ContextLog(ctx, "Files in the Downloads directory: ", string(out))

	dumpStartTimeStr := time.Now().Format("2006-01-02 15:04:05")
	waitForDownloadButton := ui.WithTimeout(5 * time.Second).WaitUntilExists(webRTCDownloadButton)
	if err := uiauto.Combine("invoke the button for the dump download",
		// Wait for |createDumpSection| node to appear to ensure
		// the following UI operations can be successfully applied.
		ui.WaitUntilExists(createDumpSection),
		uiauto.IfFailThen(
			waitForDownloadButton,
			ui.DoDefaultUntil(createDumpSection, waitForDownloadButton),
		),
		ui.DoDefault(webRTCDownloadButton),
	)(ctx); err != nil {
		return "", err
	}

	downloadStartTime := time.Now()
	// Assume WebRTC dump file name should start with "webrtc".
	const webRTCFileNamePrefix = "webrtc"
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		findFileCmd := fmt.Sprintf("find %s -name '%s*.txt' -newermt '%s'", downloadsPath, webRTCFileNamePrefix, dumpStartTimeStr)
		out, err := testexec.CommandContext(ctx, "bash", "-c", findFileCmd).Output()
		if err != nil {
			return errors.Wrapf(err, "find command %s failed", findFileCmd)
		}
		filePath := strings.TrimSpace(string(out))
		if len(filePath) == 0 {
			return errors.New("file not found")
		}
		dumpFilePath = filePath
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: 3 * time.Second}); err != nil {
		return "", errors.Wrap(err, "failed to find webrtc dump file in Downloads folder")
	}
	testing.ContextLog(ctx, "Downloaded WebRTC dump file in ", time.Since(downloadStartTime))

	return dumpFilePath, nil
}

type videoCodec float64

const (
	vp8 videoCodec = 0
	vp9 videoCodec = 1
)

// ReportWebRTCInternals reports info from a WebRTC internals dump to performance metrics.
// If a non-nil error is returned, all peer connections that were fully validated before
// the error was encountered are still reported.
func ReportWebRTCInternals(pv *perf.Values, dump []byte, numBots int, present bool) error {
	var webRTC webrtcinternals.Dump
	if err := json.Unmarshal(dump, &webRTC); err != nil {
		return errors.Wrap(err, "failed to unmarshal WebRTC internals dump")
	}

	expectedConns := 1
	expectedScreenshareConns := 0
	if present {
		expectedConns = 2
		expectedScreenshareConns = 1
	}

	if numConns := len(webRTC.PeerConnections); numConns != expectedConns {
		return errors.Errorf("unexpected number of peer connections: got %d; want %d", numConns, expectedConns)
	}

	numScreenshareConns := 0
	for connID, peerConn := range webRTC.PeerConnections {
		byType := peerConn.Stats.BuildIndex()
		inTotalCount, inScreenshareCount, err := ReportVideoStreams(pv, byType["inbound-rtp"], "framesReceived", ".Inbound", "bot%02d")
		if err != nil {
			return errors.Wrapf(err, "failed to report inbound-rtp video streams in peer connection %v", connID)
		}
		outTotalCount, outScreenshareCount, err := ReportVideoStreams(pv, byType["outbound-rtp"], "framesSent", ".Outbound", "stream%d")
		if err != nil {
			return errors.Wrapf(err, "failed to report outbound-rtp video streams in peer connection %v", connID)
		}

		if inScreenshareCount != 0 {
			return errors.Errorf("unexpected number of inbound-rtp screenshare video streams in peer connection %v; got %d, want 0", connID, inScreenshareCount)
		}
		if outTotalCount == 0 {
			return errors.Errorf("found no outbound-rtp video streams in peer connection %v", connID)
		}
		expectedInTotalCount := 0
		switch outScreenshareCount {
		case 0:
			expectedInTotalCount = numBots
		case outTotalCount:
			numScreenshareConns++
		default:
			return errors.Errorf("found %d screenshare(s) among %d outbound-rtp video streams in peer connection %v, expected all or none", outScreenshareCount, outTotalCount, connID)
		}
		if inTotalCount != expectedInTotalCount {
			return errors.Errorf("unexpected number of inbound-rtp video streams in peer connection %v; got %d, want %d", connID, inTotalCount, expectedInTotalCount)
		}
	}

	if numScreenshareConns != expectedScreenshareConns {
		return errors.Errorf("unexpected number of screenshare peer connections; got %d, want %d", numScreenshareConns, expectedScreenshareConns)
	}

	return nil
}

// ReportVideoStreams reports info from a webrtcinternals.StatsIndexByStatsID to performance
// metrics. Returns the number of active video streams, and how many of them are screenshares.
func ReportVideoStreams(pv *perf.Values, byID webrtcinternals.StatsIndexByStatsID, framesTransmittedAttribute, directionSuffix, variantFormat string) (int, int, error) {
	totalCount := 0
	screenshareCount := 0
	for id, byAttribute := range byID {
		kindTimeline, ok := byAttribute["kind"]
		if !ok {
			return 0, 0, errors.Errorf("no kind attribute for %q", id)
		}
		kind, err := kindTimeline.Collapse()
		if err != nil {
			return 0, 0, errors.Errorf("failed to collapse timeline of kind attribute for %q", id)
		}
		if kind != "video" {
			continue
		}

		framesTransmittedTimeline, ok := byAttribute[framesTransmittedAttribute]
		if !ok {
			return 0, 0, errors.Errorf("no %s attribute for %q", framesTransmittedAttribute, id)
		}
		if len(framesTransmittedTimeline) == 0 {
			return 0, 0, errors.Errorf("no values for %s attribute for %q", framesTransmittedAttribute, id)
		}
		if framesTransmittedTimeline[len(framesTransmittedTimeline)-1] == 0 {
			continue
		}

		screenShareSuffix := ""
		if contentTypeTimeline, ok := byAttribute["contentType"]; ok {
			contentType, err := contentTypeTimeline.Collapse()
			if err != nil {
				return 0, 0, errors.Errorf("failed to collapse timeline of contentType attribute for %q", id)
			}
			if contentType == "screenshare" {
				screenShareSuffix = ".Screenshare"
				screenshareCount++
			}
		}

		for _, config := range []struct {
			attribute       string
			reporter        func(interface{}) (float64, error)
			attributeSuffix string
			unit            string
		}{
			{"frameWidth", reportFloat64, ".frameWidth", "px"},
			{"frameHeight", reportFloat64, ".frameHeight", "px"},
			{"framesPerSecond", reportFloat64, ".framesPerSecond", "fps"},
			{"[codec]", reportVideoCodec, ".codec", "unitless"},
		} {
			timeline, ok := byAttribute[config.attribute]
			if !ok {
				continue
			}

			var report []float64
			for _, value := range timeline {
				metric, err := config.reporter(value)
				if err != nil {
					return 0, 0, errors.Wrapf(err, "failed to represent %s attribute for %q as performance metric", config.attribute, id)
				}
				report = append(report, metric)
			}

			pv.Set(perf.Metric{
				Name:      fmt.Sprintf("WebRTCInternals.Video%s%s%s", screenShareSuffix, directionSuffix, config.attributeSuffix),
				Variant:   fmt.Sprintf(variantFormat, totalCount),
				Unit:      config.unit,
				Direction: perf.BiggerIsBetter,
				Multiple:  true,
			}, report...)
		}
		totalCount++
	}
	return totalCount, screenshareCount, nil
}

// reportFloat64 simply typecasts from interface{} to float64.
func reportFloat64(value interface{}) (float64, error) {
	report, ok := value.(float64)
	if !ok {
		return 0, errors.Errorf("%v is not of type float64", value)
	}
	return report, nil
}

// reportVideoCodec parses a video codec description from a WebRTC internals dump, and
// represents the video codec as float64 so it can be reported to a performance metric.
func reportVideoCodec(value interface{}) (float64, error) {
	description, ok := value.(string)
	if !ok {
		return 0, errors.Errorf("%v is not of type string", value)
	}

	if strings.HasPrefix(description, "VP8") {
		return float64(vp8), nil
	}
	if strings.HasPrefix(description, "VP9") {
		return float64(vp9), nil
	}
	return 0, errors.Errorf("unrecognized video stream codec: %q", description)
}
