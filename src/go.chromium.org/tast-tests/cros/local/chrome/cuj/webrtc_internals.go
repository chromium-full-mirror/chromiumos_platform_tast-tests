// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/webrtcinternals"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const createDumpSectionName = "Create Dump"

var (
	createDumpSectionReg                    = regexp.MustCompile("(Create Dump)|(Create a WebRTC-Internals dump)")
	createDumpSection                       = nodewith.NameRegex(createDumpSectionReg).Role(role.DisclosureTriangle)
	webRTCRootWebArea                       = nodewith.Name("WebRTC Internals").Role(role.RootWebArea)
	webRTCDownloadButton                    = nodewith.NameContaining("Download").Role(role.Button).Ancestor(webRTCRootWebArea)
	createDiagnosticAudioRecordingsSection  = nodewith.Name("Create diagnostic audio recordings").Role(role.DisclosureTriangle)
	enableDiagnosticAudioRecordingsCheckbox = nodewith.Name("Enable diagnostic audio recordings").Role(role.CheckBox)
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

	dumpWebRTCFile := func(ctx context.Context) error {
		dumpStartTime := time.Now()
		testing.ContextLog(ctx, "Start to dump WebRTC file at ", dumpStartTime)
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
			return err
		}

		downloadStartTime := time.Now()
		// Assume WebRTC dump file name should start with "webrtc".
		const webRTCFileName = "webrtc*.txt"
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			files, err := filepath.Glob(filepath.Join(downloadsPath, webRTCFileName))
			if err != nil {
				return errors.Wrap(err, "failed to glob webrtc file")
			}
			if len(files) == 0 {
				return errors.New("file not found")
			}
			for _, file := range files {
				fState, err := os.Stat(file)
				if err != nil {
					continue
				}
				if fState.ModTime().After(dumpStartTime) {
					dumpFilePath = file
					break
				}
			}
			if len(dumpFilePath) == 0 {
				return errors.Errorf("cannot find file modified after %v", dumpStartTime)
			}
			return nil
		}, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 3 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to find webrtc dump file in Downloads folder")
		}
		testing.ContextLog(ctx, "Downloaded WebRTC dump file in ", time.Since(downloadStartTime))
		return nil
	}

	// Sometimes download button might not be clicked successfully
	// and some DUTs need more time to download the file. Add retries
	// to mitigate the problem.
	if err := uiauto.Retry(3, dumpWebRTCFile)(ctx); err != nil {
		return "", errors.Wrap(err, "failed to dump WebRTC file")
	}

	return dumpFilePath, nil
}

// DumpDiagnosticAudioRecordings downloads aecdump files from
// chrome://webrtc-internals to the Downloads path of the current user.
func DumpDiagnosticAudioRecordings(ctx context.Context, tconn *chrome.TestConn) (err error) {
	ui := uiauto.New(tconn)
	dumpAudioFile := func(ctx context.Context) error {
		if err := uiauto.NamedCombine("Enable diagnostic audio recordings",
			ui.WaitUntilExists(createDiagnosticAudioRecordingsSection),
			ui.DoDefault(createDiagnosticAudioRecordingsSection),
			ui.WaitUntilExists(enableDiagnosticAudioRecordingsCheckbox),
			ui.DoDefault(enableDiagnosticAudioRecordingsCheckbox),
		)(ctx); err != nil {
			return err
		}

		// Find the files app dialog.
		saver, err := filesapp.App(ctx, tconn, filesapp.FileSaverPseudoAppID)
		if err != nil {
			return err
		}
		saver = saver.WithTimeout(10 * time.Second)
		saveButton := nodewith.Role(role.Button).Name("Save")
		if err := uiauto.NamedCombine("Save audio_debug",
			saver.OpenDir("Downloads", "Downloads"),
			saver.WaitUntilExists(saveButton),
			saver.LeftClick(saveButton),
		)(ctx); err != nil {
			return errors.Wrap(err, "cannot select diagnostic audio recordings filename")
		}
		testing.ContextLog(ctx, "Enable diagnostic audio recordings")
		return nil
	}

	if err := uiauto.Retry(3, dumpAudioFile)(ctx); err != nil {
		return errors.Wrap(err, "failed to enable diagnostic audio recordings")
	}

	return nil
}

// CleanupDiagnosticAudioRecordings deletes audio diagnostic recordings.
func CleanupDiagnosticAudioRecordings(ctx context.Context, path string) {
	deleteFileWithPattern(ctx, filepath.Join(path, "*.wav"))
	deleteFileWithPattern(ctx, filepath.Join(path, "*.aecdump"))
}

// findLargestFileWithPattern returns the path of the largest file with the
// given pattern.
func findLargestFileWithPattern(pattern string) (file string, size int64, err error) {
	files, err := filepath.Glob(pattern)
	if err != nil {
		return "", 0, errors.Wrap(err, "failed to glob files")
	}
	if len(files) == 0 {
		return "", 0, errors.New("no files found")
	}

	var largestFileSize int64
	var largestFile string
	for _, file := range files {
		fState, err := os.Stat(file)
		if err != nil {
			continue
		}
		if fState.Size() > largestFileSize {
			largestFileSize = fState.Size()
			largestFile = file
		}
	}

	if largestFile == "" {
		return "", 0, errors.Errorf("cannot find file with pattern %q", pattern)
	}
	return largestFile, largestFileSize, nil
}

// deleteFileWithPattern deletes files with the given pattern.
func deleteFileWithPattern(ctx context.Context, pattern string) (err error) {
	files, err := filepath.Glob(pattern)
	if err != nil {
		return errors.Wrap(err, "failed to glob files")
	}
	for _, file := range files {
		err := os.Remove(file)
		if err != nil {
			testing.ContextLog(ctx, "Error deleting file:", file, err)
		} else {
			testing.ContextLog(ctx, "Deleted:", file)
		}
	}
	return nil
}

// CalculateEchoRMS calculates the root mean square (RMS) amplitude of the echo from
// the meeting .aecdump files.
func CalculateEchoRMS(ctx context.Context, downloadsPath string) (float64, error) {
	aecDump, largestAECDumpSize, err := findLargestFileWithPattern(filepath.Join(downloadsPath, "*.aecdump"))
	if err != nil {
		return -1, errors.Wrap(err, "cannot find aecdump file")
	}
	testing.ContextLogf(ctx, "aecdump file: %s (Size: %d bytes)", aecDump, largestAECDumpSize)

	// Unpack the aecdump file.
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return -1, errors.Wrap(err, "failed to create temp dir")
	}
	defer os.RemoveAll(tempDir)
	aecDumpCmd := testexec.CommandContext(ctx, "unpack_aecdump", aecDump)
	aecDumpCmd.Dir = tempDir

	if _, err := aecDumpCmd.Output(); err != nil {
		return -1, errors.Wrap(err, "cannot run unpack_aecdump")
	}
	refOut, refOutSize, err := findLargestFileWithPattern(filepath.Join(tempDir, "ref_out*.wav"))
	if err != nil {
		return -1, errors.Wrap(err, "cannot find ref_out.wav")
	}
	testing.ContextLogf(ctx, "ref_out file: %s (Size: %d bytes)", refOut, refOutSize)
	// Trim 5 seconds from the beginning as we expect some echos happen at the
	// beginning.
	trimmedRefOut := filepath.Join(tempDir, "ref_out_trim.wav")
	audio.TrimFileFrom(ctx, refOut, trimmedRefOut, 5*time.Second)
	rms, err := audio.GetRmsAmplitudeFromWav(ctx, trimmedRefOut)
	if err != nil {
		return -1, errors.Wrap(err, "cannot get rms amplitude")
	}
	// dBFS = 20 * log10 (RMS / Reference Level).
	// For 32 bit float PCM, the `Reference Level` is 1.
	dbfs := 20 * math.Log10(rms)
	testing.ContextLogf(ctx, "dbfs: %f dB", dbfs)
	return dbfs, nil
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
	orderedIDs := make([]string, 0)
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
		if framesTransmittedTimeline[len(framesTransmittedTimeline)-1] == float64(0) {
			continue
		}

		orderedIDs = append(orderedIDs, id)
	}

	sort.Slice(orderedIDs, func(i, j int) bool {
		// Sort by number of items in the frames transmitted timeline as a simple
		// proxy for length of time each stream was active. In descending order so
		// the bots with longest time spent in the meeting sort earlier.
		return len(byID[orderedIDs[i]][framesTransmittedAttribute]) > len(byID[orderedIDs[j]][framesTransmittedAttribute])
	})

	type reportableMetric struct {
		attribute       string
		reporter        func(interface{}) (float64, error)
		attributeSuffix string
		unit            string
		direction       perf.Direction
	}

	metrics := []reportableMetric{
		{"frameWidth", reportFloat64, ".frameWidth", "px", perf.BiggerIsBetter},
		{"frameHeight", reportFloat64, ".frameHeight", "px", perf.BiggerIsBetter},
		{"framesDecoded", reportFloat64, ".framesDecoded", "frames", perf.BiggerIsBetter},
		{"framesDropped", reportFloat64, ".framesDropped", "frames", perf.SmallerIsBetter},
		{"framesPerSecond", reportFloat64, ".framesPerSecond", "fps", perf.BiggerIsBetter},
		{"freezeCount", reportFloat64, ".freezeCount", "count", perf.SmallerIsBetter},
		{"totalFreezesDuration", reportFloat64, ".totalFreezesDuration", "s", perf.SmallerIsBetter},
		{"[codec]", reportVideoCodec, ".codec", "unitless", perf.BiggerIsBetter},
	}

	aggregates := map[string]float64{
		"framesDecoded":        0,
		"framesDropped":        0,
		"freezeCount":          0,
		"totalFreezesDuration": 0,
	}

	for _, id := range orderedIDs {
		byAttribute := byID[id]
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

		for _, config := range metrics {
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

			if _, ok := aggregates[config.attribute]; ok {
				// The timeline values for the metrics we are trying to
				// aggregate are always increasing. Thus, the last value in the
				// list is always the total value of that unit. For example,
				// the last value in the framesDropped timeline is the total
				// number of dropped frames.
				aggregates[config.attribute] += report[len(report)-1]
			}

			pv.Set(perf.Metric{
				Name:      fmt.Sprintf("WebRTCInternals.Video%s%s%s.%s", screenShareSuffix, directionSuffix, config.attributeSuffix, fmt.Sprintf(variantFormat, totalCount)),
				Unit:      config.unit,
				Direction: config.direction,
				Multiple:  true,
			}, report...)
		}
		totalCount++
	}

	for _, config := range metrics {
		// Create a metric in the form:
		// WebRTCInternals.Video.{Inbound, Outbound}.{title-cased metric name}
		if aggregate, ok := aggregates[config.attribute]; ok {
			pv.Set(perf.Metric{
				Name:      fmt.Sprintf("WebRTCInternals.Video%s.%s", directionSuffix, cases.Title(language.Und, cases.NoLower).String(config.attribute)),
				Unit:      config.unit,
				Direction: config.direction,
			}, aggregate)
		}
	}

	if aggregates["framesDecoded"] > 0 {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("WebRTCInternals.Video%s.PercentDroppedFrames", directionSuffix),
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}, aggregates["framesDropped"]/aggregates["framesDecoded"])
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
