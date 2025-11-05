// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const createDumpSectionName = "Create Dump"

var (
	createDumpSectionReg                    = regexp.MustCompile("(Create Dump)|(Create a WebRTC-Internals dump)")
	createDumpSection                       = nodewith.NameRegex(createDumpSectionReg).Role(role.DisclosureTriangle)
	webRTCRootWebArea                       = nodewith.Name("WebRTC Internals").Role(role.RootWebArea)
	webRTCDownloadButton                    = nodewith.Name("Download the \"rtcstats dump\"").Role(role.Button).Ancestor(webRTCRootWebArea)
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

// DumpWebRTCInternals downloads a dump from chrome://webrtc-internals and
// returns the file path. This function assumes that chrome://webrtc-internals
// is already shown, with the Create Dump section expanded.
func DumpWebRTCInternals(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, conn *chrome.Conn, username string) (dumpFilePath string, err error) {
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
		// WebRTC dump file name should be "rtcstats_dump.gz".
		const webRTCFileName = "rtcstats_dump.gz"
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
			if err := conn.Eval(ctx, "location.reload()", nil); err != nil {
				return errors.Wrap(err, "failed to reload the webrtc-internals page")
			}
			if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
				testing.ContextLog(ctx, "Failed to wait for the webrtc-internals page to quiesce: ", err)
			}
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
	vp8 videoCodec = iota
	vp9
	av1
)

// ReportWebRTCInternals reports info from a WebRTC internals dump to performance metrics.
// If a non-nil error is returned, all peer connections that were fully validated before
// the error was encountered are still reported.
func ReportWebRTCInternals(ctx context.Context, dump []byte, meetingCode string, numBots int, enterpriseEffects, present bool) (*perf.Values, error) {
	stats, err := DecodeRTCStats(dump)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode RTC stats")
	}

	var (
		inCountError, outCountErr error
		foundInbound              bool
	)
	expectedConns := 1
	expectedScreenshareConns := 0
	if present {
		expectedConns = 2
		expectedScreenshareConns = 1
	}

	numScreenshareConns := 0
	outboundVideoStream := 0

	pv := perf.NewValues()

	var sessionIDs []string
	// Collect all SessionIDs for connections that were created for a specific meeting.
	for _, s := range stats {
		if s.EventType == "create" && strings.Contains(s.URL, meetingCode) {
			sessionIDs = append(sessionIDs, s.SessionID)
		}
	}

	// Create a map where each key is a SessionID and the value is a slice of stats data.
	statsData := make(map[string][]map[string]interface{}, len(sessionIDs))
	for _, s := range stats {
		if s.EventType != "getStats" || !slices.Contains(sessionIDs, s.SessionID) {
			continue
		} else {
			statsData[s.SessionID] = append(statsData[s.SessionID], s.Data)
		}
	}
	numPeerConns := len(statsData)

	for connID, statsList := range statsData {
		var inTotalCount, inScreenshareCount int
		if !foundInbound {
			inTotalCount, inScreenshareCount, err = ReportVideoStreams(pv, statsList, "inbound-rtp", "framesReceived", ".Inbound", "bot%02d")
			if err != nil {
				return nil, errors.Wrapf(err, "failed to report inbound-rtp video streams in peer connection %v", connID)
			}
			if inScreenshareCount != 0 {
				return nil, errors.Errorf("unexpected number of inbound-rtp screenshare video streams in peer connection %v; got %d, want 0", connID, inScreenshareCount)
			}
		}
		outTotalCount, outScreenshareCount, err := ReportVideoStreams(pv, statsList, "outbound-rtp", "framesSent", ".Outbound", "stream%d")
		if err != nil {
			return nil, errors.Wrapf(err, "failed to report outbound-rtp video streams in peer connection %v", connID)
		}

		if outTotalCount == 0 {
			testing.ContextLog(ctx, "Found no outbound-rtp video streams in peer connection ", connID)
			outCountErr = errors.Errorf("found no outbound-rtp video streams in peer connection %v", connID)
			continue
		} else {
			outboundVideoStream++
		}
		expectedInTotalCount := 0
		switch outScreenshareCount {
		case 0: // This is the video chat connection.
			// Sometimes when the connection is unstable, there may be multiple peer connections.
			// Return failure only if none of the connections have correct inbound video data.

			// Continue if inbound-rtp video streams has been found in the peer connection.
			if foundInbound {
				continue
			}
			expectedInTotalCount = numBots

			testing.ContextLogf(ctx, "Found %v inbound-rtp video streams", inTotalCount)
			// If an enterprise account turns on effects, it may generate 1~2 inbound-rtp video
			// streams for the self view of sending client, in particular on lower-end devices.
			if enterpriseEffects {
				if inTotalCount < expectedInTotalCount || inTotalCount > expectedInTotalCount+2 {
					inCountError = errors.Errorf("unexpected number of inbound-rtp video streams in peer connection %v; got %d, expected to be in range [%d, %d]", connID, inTotalCount, expectedInTotalCount, expectedInTotalCount+2)
				} else {
					inCountError = nil
				}
			} else {
				if inTotalCount != expectedInTotalCount {
					inCountError = errors.Errorf("unexpected number of inbound-rtp video streams in peer connection %v; got %d, want %d", connID, inTotalCount, expectedInTotalCount)
				} else {
					inCountError = nil
				}
			}
			if inCountError == nil {
				foundInbound = true
			}
		case outTotalCount: // This is the screen share connection.
			numScreenshareConns++
			if inTotalCount != expectedInTotalCount {
				return nil, errors.Errorf("unexpected number of inbound-rtp video streams in screenshare peer connection %v; got %d, want %d", connID, inTotalCount, expectedInTotalCount)
			}
		default:
			return nil, errors.Errorf("found %d screenshare(s) among %d outbound-rtp video streams in peer connection %v, expected all or none", outScreenshareCount, outTotalCount, connID)
		}
	}
	if outboundVideoStream < expectedConns && outCountErr != nil {
		return nil, outCountErr
	}
	if inCountError != nil {
		return nil, inCountError
	}
	if numPeerConns < expectedConns {
		return nil, errors.Errorf("unexpected number of peer connections; got %d, want %d", numPeerConns, expectedConns)
	} else if numPeerConns > expectedConns {
		testing.ContextLogf(ctx, "Got more peer connections; got %d, want %d", numPeerConns, expectedConns)
	}

	if numScreenshareConns < expectedScreenshareConns {
		return nil, errors.Errorf("unexpected number of screenshare peer connections; got %d, want %d", numScreenshareConns, expectedScreenshareConns)
	} else if numScreenshareConns > expectedScreenshareConns {
		testing.ContextLogf(ctx, "Got more screenshare peer connections; got %d, want %d", numScreenshareConns, expectedScreenshareConns)
	}
	return pv, nil
}

// ReportVideoStreams reports info from a stats list to performance metrics.
// Returns the number of active video streams, and how many of them are screenshares.
func ReportVideoStreams(pv *perf.Values, statsList []map[string]interface{}, dataType, framesTransmittedAttribute, directionSuffix, variantFormat string) (int, int, error) {
	screenshareCount := 0
	screenShareSuffix := ""
	streamsByID := make(map[string][]interface{})
	orderedStreamIDs := make([]string, 0)

	for _, statMap := range statsList {
		for _, frameItem := range statMap {
			frameData, ok := frameItem.(map[string]interface{})
			if !ok {
				continue
			}
			streamID, ok := frameData["id"].(string)
			if !ok {
				continue
			}
			if kind, ok := frameData["kind"].(string); !ok || kind != "video" {
				continue
			}
			if t, ok := frameData["type"].(string); !ok || t != dataType {
				continue
			}
			framesTransmittedVal := frameData[framesTransmittedAttribute]
			framesTransmitted, err := reportFloat64(framesTransmittedVal)
			if err != nil {
				return 0, 0, errors.Wrapf(err, "failed to parse string %q to float64", framesTransmittedVal)
			}
			if framesTransmitted <= 0 {
				continue
			}
			if streamsByID[streamID] != nil {
				streamsByID[streamID] = append(streamsByID[streamID], frameItem)
				continue
			}
			// Initialize the stream record for the first time.
			streamsByID[streamID] = []interface{}{frameItem}
			orderedStreamIDs = append(orderedStreamIDs, streamID)
			contentType, ok := frameData["contentType"].(string)
			if ok && contentType == "screenshare" {
				screenShareSuffix = ".Screenshare"
				screenshareCount++
			}
		}
	}

	sort.Slice(orderedStreamIDs, func(i, j int) bool {
		// Sort by the number of frames collected as a proxy for active duration.
		// In descending order so the bots with longest time spent in the meeting
		// sort earlier.
		return len(streamsByID[orderedStreamIDs[i]]) > len(streamsByID[orderedStreamIDs[j]])
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
		{"framesEncoded", reportFloat64, ".framesEncoded", "frames", perf.BiggerIsBetter},
		{"framesDropped", reportFloat64, ".framesDropped", "frames", perf.SmallerIsBetter},
		{"framesPerSecond", reportFloat64, ".framesPerSecond", "fps", perf.BiggerIsBetter},
		{"freezeCount", reportFloat64, ".freezeCount", "count", perf.SmallerIsBetter},
		{"totalFreezesDuration", reportFloat64, ".totalFreezesDuration", "s", perf.SmallerIsBetter},
		{"[codec]", reportVideoCodec, ".codec", "unitless", perf.BiggerIsBetter},
		{"totalDecodeTime", reportFloat64, ".totalDecodeTime", "s", perf.SmallerIsBetter},
		{"totalEncodeTime", reportFloat64, ".totalEncodeTime", "s", perf.SmallerIsBetter},
		{"powerEfficientDecoder", reportPowerEfficient, ".powerEfficientDecoder", "unitless", perf.BiggerIsBetter},
		{"powerEfficientEncoder", reportPowerEfficient, ".powerEfficientEncoder", "unitless", perf.BiggerIsBetter},
	}

	aggregates := map[string]float64{
		"framesDecoded":        0,
		"framesEncoded":        0,
		"framesDropped":        0,
		"freezeCount":          0,
		"totalFreezesDuration": 0,
		"totalDecodeTime":      0,
		"totalEncodeTime":      0,
	}

	timeline := make(map[string]map[string][]float64)
	implementationList := make(map[string]string)
	implementationAttribute := "decoderImplementation"
	if directionSuffix == ".Outbound" {
		implementationAttribute = "encoderImplementation"
	}

	for id, dataList := range streamsByID {
		for _, v := range dataList {
			m, ok := v.(map[string]interface{})
			if !ok {
				continue
			}

			if _, ok := timeline[id]; !ok {
				timeline[id] = make(map[string][]float64)
			}

			for _, config := range metrics {
				value, ok := m[config.attribute]
				if !ok || value == nil {
					continue
				}
				// Skip if the timeline value is nil.
				if value == nil {
					continue
				}
				metric, err := config.reporter(value)
				if err != nil {
					continue
				}
				timeline[id][config.attribute] = append(timeline[id][config.attribute], metric)
			}

			implementation, ok := m[implementationAttribute].(string)
			if !ok || implementation == "" {
				continue
			}
			if implementationList[id] == "" {
				implementation = perf.InvalidNameRe.ReplaceAllString(implementation, "_")
				implementationList[id] = implementation
			}
		}
	}

	for botNumber, id := range orderedStreamIDs {
		variantSuffix := fmt.Sprintf(variantFormat, botNumber)
		metricsMap := timeline[id]
		for _, config := range metrics {
			report, ok := metricsMap[config.attribute]
			if !ok || len(report) == 0 {
				continue
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
				Name:      fmt.Sprintf("WebRTCInternals.Video%s%s%s.%s", screenShareSuffix, directionSuffix, config.attributeSuffix, variantSuffix),
				Unit:      config.unit,
				Direction: config.direction,
				Multiple:  true,
			}, report...)
		}

		// Record the implementation in the name of the metric:
		// Example: WebRTCInternals.Video.Screenshare.Outbound.encoderImplementation.SimulcastEncoderAdapter__libvpx__libvpx_.stream0
		pv.Set(perf.Metric{
			Name: fmt.Sprintf("WebRTCInternals.Video%s%s.%s.%s.%s", screenShareSuffix, directionSuffix, implementationAttribute, implementationList[id], variantSuffix),
			Unit: "unitless",
		}, 0)
	}

	for _, config := range metrics {
		// Create a metric in the form:
		// WebRTCInternals.Video.{Inbound, Outbound}.{title-cased metric name}
		if aggregate, ok := aggregates[config.attribute]; ok && aggregate > 0 {
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
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("WebRTCInternals.Video%s.AverageDecodeTime", directionSuffix),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, aggregates["totalDecodeTime"]/aggregates["framesDecoded"]*1000)
	}
	if aggregates["framesEncoded"] > 0 {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("WebRTCInternals.Video%s%s.AverageEncodeTime", screenShareSuffix, directionSuffix),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, aggregates["totalEncodeTime"]/aggregates["framesEncoded"]*1000)
	}
	totalCount := len(orderedStreamIDs)
	return totalCount, screenshareCount, nil
}

// reportFloat64 simply typecasts from interface{} to float64.
func reportFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return value.(float64), nil
	case string:
		// Translate string to float64.
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to parse string %q to float64", v)
		}
		return f, nil
	default:
		return 0, errors.Errorf("%v is not of type float64", value)
	}
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
	if strings.HasPrefix(description, "AV1") {
		return float64(av1), nil
	}
	return 0, errors.Errorf("unrecognized video stream codec: %q", description)
}

func reportPowerEfficient(value interface{}) (float64, error) {
	powerEfficient, ok := value.(bool)
	if !ok {
		return 0, errors.Errorf("%v is not of type bool", value)
	}
	if powerEfficient {
		return 1, nil
	}
	return 0, nil
}

// ReadWebRTCFile reads a gzipped WebRTC internals dump file and returns
// its decompressed contents.
func ReadWebRTCFile(filename string) ([]byte, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %q", filename)
	}

	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "failed to open gzip reader")
	}
	defer r.Close()

	return io.ReadAll(r)
}

// RTCStatsLine represents a single parsed line from a WebRTC internals dump.
// Each line corresponds to a recorded event with its session metadata and
// data payload.
type RTCStatsLine struct {
	EventType string
	SessionID string
	Data      map[string]interface{}
	URL       string
}

// DecodeRTCStats parses raw WebRTC internals dump data into a structured
// slice of RTCStatsLine. Each line in the dump is expected to be a JSON
// array containing event metadata.
func DecodeRTCStats(data []byte) ([]RTCStatsLine, error) {
	var results []RTCStatsLine
	scanner := bufio.NewScanner(bytes.NewReader(data))
	buf := make([]byte, 0, 10*1024*1024)
	scanner.Buffer(buf, 50*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] != '[' {
			continue
		}

		var parsedArray []interface{}
		if err := json.Unmarshal(line, &parsedArray); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal")
		}

		if len(parsedArray) < 4 {
			continue
		}

		stats := RTCStatsLine{}
		if eventType, ok := parsedArray[0].(string); ok {
			stats.EventType = eventType
		}
		if sessionID, ok := parsedArray[1].(string); ok {
			stats.SessionID = sessionID
		}
		if data, ok := parsedArray[2].(map[string]interface{}); ok {
			stats.Data = data
		}
		if url, ok := parsedArray[3].(string); ok {
			stats.URL = url
		}

		results = append(results, stats)
	}
	return results, scanner.Err()
}
