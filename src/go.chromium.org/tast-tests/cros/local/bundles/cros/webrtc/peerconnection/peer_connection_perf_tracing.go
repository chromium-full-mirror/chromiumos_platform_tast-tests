// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package peerconnection provides code for WebRTC's RTCPeerConnection performance tests.
package peerconnection

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// TraceConfigFile is a perfetto configuration to collect chrome trace events for a webrtc peerconnection call.
	TraceConfigFile = "perfetto_peerconnection_trace.pbtxt"
	// ChromeTraceSQLFile is a SQL file to extract the chrome events of hardware video encoding and decoding.
	ChromeTraceSQLFile = "peerconnection_chrome_event_trace.sql"

	// Time to collect tracing.
	tracingMeasuring = 10 * time.Second

	// The string to indicate no suffix for a reported performance name.
	noPerfNameSuffix = ""
)

// recordTracing traces chrome events about hardware video decoding and encoding
// in a webrtc peerconnection using perfetto with TraceConfigFile and ChromeTraceSQLFile.
// If a user specifies -var=webrtc.RTCPeerConnectionPerf.SaveTracing=true, the trace data is saved.
func recordTracing(ctx context.Context, s *testing.State) (traceOut [][]string, err error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	saveTracing := false
	if val, ok := s.Var("webrtc.RTCPeerConnectionPerf.SaveTracing"); ok {
		saveTracing = (strings.ToLower(val) == "true")
	}

	testing.ContextLog(ctx, "Tracing chrome events")
	// Record system events for |tracingMeasuring|.
	var sess *tracing.Session
	if saveTracing {
		sess, err = tracing.StartSession(ctx, s.DataPath(TraceConfigFile),
			tracing.WithTraceDataPath(filepath.Join(
				s.OutDir(),
				fmt.Sprintf("perfetto-chrome_trace_events_%d.pb", time.Now().Unix()))),
			tracing.WithCompression())
	} else {
		sess, err = tracing.StartSession(ctx, s.DataPath(TraceConfigFile))
	}
	if err != nil {
		return traceOut, errors.Wrap(err, "failed to start tracing")
	}
	defer sess.Finalize(cleanupCtx)
	// Stop tracing even if context deadline exceeds during sleep.
	stopped := false
	defer func() {
		if !stopped {
			sess.Stop(ctx)
		}
	}()
	// GoBigSleepLint: sleep to collect tracing events
	if err := testing.Sleep(ctx, tracingMeasuring); err != nil {
		return traceOut, errors.Wrap(err, "failed to sleep to wait for the tracing session")
	}
	stopped = true
	if err := sess.Stop(ctx); err != nil {
		return traceOut, errors.Wrap(err, "failed to stop tracing")
	}
	testing.ContextLog(ctx, "Completed tracing events")

	traceOut, err = sess.RunQuery(ctx, s.DataPath(ChromeTraceSQLFile))
	if err != nil {
		return traceOut, errors.Wrap(err, "failed in querying")
	}

	return traceOut, nil
}

type chromeEventInfo struct {
	// The name of chrome trace event.
	name string
	// The timestamp that the trace event is invoked.
	eventTS int64
	// The corresponded timestamp that related chrome trace events have.
	// It is the input VideoFrame timestamp in encoding and input DecoderBuffer
	// timestamp in decoding.
	keyTS int64
	// The duration of the trace event.
	duration int64
	// The integer/strings arguments that are recorded in a trace event, for instance,
	// "bitstream_buffer_id" in BitstreamBufferReady().
	intArgs map[string]int64
	strArgs map[string]string
}

// formatTraceOutput formats the results of the traced data, [][]string, so that it can be easily processed later, []chromeEventInfo.
// If chrome trace event has multiple arguments, perfetto records the same multiple events each of which has
// associated one argument. For example, VAVEA::ImageProcessor::BlitSurface has two arguments, "debug.dest_visible_rect" and "debug.source_visible_rect",
// so the two slices are shown in the trace data.
// "VAVEA::ImageProcessor::BlitSurface",258102008355180,313000,"debug.dest_visible_rect","string","[NULL]","0,0 640x360"
// "VAVEA::ImageProcessor::BlitSurface",258102008355180,313000,"debug.source_visible_rect","string","[NULL]","0,0 1280x720"
func formatTraceOutput(traceOut [][]string) (events []chromeEventInfo, err error) {
	var prevName string
	var prevEventTS int64
	for _, out := range traceOut[1:] {
		// Skip, the first line,
		// "name","dur","ts","key","value_type","int_value","string_value"
		const (
			nameIndex = iota
			eventTSIndex
			durIndex
			argKeyIndex
			argValueTypeIndex
			argIntValueIndex
			argStrValueIndex
		)
		name := out[nameIndex]
		eventTS, err := strconv.ParseInt(out[eventTSIndex], 10, 64)
		if err != nil {
			return events, errors.Wrapf(err, "failed to parse event timestamp: %v", out[eventTSIndex])
		}

		if prevName != name || prevEventTS != eventTS {
			duration, err := strconv.ParseInt(out[durIndex], 10, 64)
			if err != nil {
				return events, errors.Wrapf(err, "failed to parse duration: %v", out[durIndex])
			}
			// keyTS is filled later from parsing arguments.
			events = append(events, chromeEventInfo{
				name:     name,
				eventTS:  eventTS,
				duration: duration,
				intArgs:  make(map[string]int64),
				strArgs:  make(map[string]string),
			})
			prevName = name
			prevEventTS = eventTS
		}

		// The timestamp used as key is recorded in arguments, basically "debug.timestamp".
		// The exception is MojoVideoDecoder::Decode and MojoVideoDecoder::OnVideoFrameDecoded, in which
		// the timestamp is recorded as a part of "buffer" and "frame", respectively.
		curEvent := &events[len(events)-1]
		argKey := out[argKeyIndex]
		argKey = strings.Replace(argKey, "debug.", "", 1)
		switch out[argValueTypeIndex] {
		case "int", "uint", "bool":
			intValue, err := strconv.ParseInt(out[argIntValueIndex], 10, 64)
			if err != nil {
				return events, errors.Wrapf(err, "failed to int value: %v", out[argIntValueIndex])
			}
			if argKey == "timestamp" {
				curEvent.keyTS = intValue
			} else {
				curEvent.intArgs[argKey] = intValue
			}
		case "string":
			curEvent.strArgs[argKey] = out[argStrValueIndex]
			if name == "MojoVideoDecoder::OnVideoFrameDecoded" && argKey == "frame" {
				// MojoVideoDecoder::OnVideoFrameDecoded() reports the timestamp by VideoFrame::AsHumanReadableString().
				// Example: format:PIXEL_FORMAT_NV12 storage_type:OPAQUE coded_size:1280x720 visible_rect:0,0 1280x720 natural_size:1280x720 timestamp:3947083370 textures: 1
				frameStr := out[argStrValueIndex]
				beginIndex := strings.Index(frameStr, "timestamp:")
				endIndex := strings.Index(frameStr, "textures:")
				// textures is not logged if a frame doesn't have a texture.
				if endIndex == -1 {
					endIndex = len(frameStr)
				}
				if beginIndex == -1 {
					return events, errors.Errorf("no timestamp in VideoFrame::AsHumanReadableString(): %v", frameStr)
				}
				beginIndex = beginIndex + len("timestamp:")
				timestampStr := strings.ReplaceAll(frameStr[beginIndex:endIndex], " ", "")
				timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
				if err != nil {
					return events, errors.Wrapf(err, "failed to timestamp in VideoFrame::AsHumanReadableString(): %v", frameStr)
				}
				curEvent.keyTS = timestamp
			} else if name == "MojoVideoDecoder::Decode" && argKey == "buffer" {
				// MojoVideoDecoder::Decode() reports the timestamp by DecoderBuffer::AsHumanReadableString().
				bufferStr := out[argStrValueIndex]
				beginIndex := len("{timestamp=")
				endIndex := strings.Index(bufferStr, "duration=")
				if endIndex == -1 {
					return events, errors.Errorf("no duration in DecoderBuffer::AsHumanReadableString(): %v", bufferStr)
				}
				timestampStr := strings.ReplaceAll(bufferStr[beginIndex:endIndex], " ", "")
				timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
				if err != nil {
					return events, errors.Wrapf(err, "failed to timestamp in DecoderBuffer::AsHumanReadableString(): %v", bufferStr)
				}
				curEvent.keyTS = timestamp
			}
		}
	}
	return events, nil
}

type chromeTraceEventResultIndex int

// This represents the reported performance time. They are two types of time.
// (1) The interval time between events
//
//	This is the difference of the event timestamp of the two events.
//	For example, we can know how long the mojo call and messaging takes by
//	the event timestamps of MojoVideoEncodeAccelerator::Encode and MojoVideoEncodeAcceleratorService::Encode.
//
// (2) The duration of an event
//
//	For example, the duration of MojoVEAService::EncodingFrameDuration event
//	is the time between MojoVEAService::Encode() and MojoVEAService::BitstreamBufferReady(),
//	in other words, the time of encoding the frame in chrome GPU process.
const (
	// encode event
	// RTCVideoEncoder::Encode - MojoVideoEncodeAccelerator::Encode
	rtcToMojoVEA chromeTraceEventResultIndex = iota
	// MojoVideoEncodeAccelerator::Encode - MojoVideoEncodeAcceleratorService::Encode
	mojoVEAToMojoVEAService
	// MojoVideoEncodeAcceleratorService::Encode - (VAVEA::EncodeTask, V4L2VEA::EncodeTask)
	mojoVEAServiceToVEA
	// (VAVEA::BitstreamBufferReady, V4L2VEA::BitstreamBufferReady) - MojoVideoEncodeAcceleratorService::BitstreamBufferReady
	veaToMojoVEAService
	// MojoVideoEncodeAcceleratorService::BitstreamBufferReady - RTCVideoEncoder::Impl::BitstreamBufferReady
	mojoVEAServiceToRTC

	// encode duration
	// RTCVideoEncoder::Impl::EncodeOneFrame, RTCVideoEncoder::Impl::EncodeOneFrameWithNativeInput.
	rtcEncodeFuncDuration
	// PlatformEncoding.Encode
	veaEncodingDuration
	// MojoVEAService::EncodingFrameDuration
	gpuEncodingDuration
	// V4L2VEA::ImageProcessor::Process, VAVEA::ImageProcessor::BlitSurface
	imageProcessDuration

	// decode event
	// MojoVideoDecoder::Decode - VideoDecoderPipeline::Decode
	mojoVDToVDPDecode
	// VideoDecoderPipeline::Decode - VideoDecoderPipeline::DecodeTask
	vdpDecodeToDecodeTask
	// VideoDecoderPipeline::DecodeTask - VideoDecoderPipeline::OnFrameDecoded
	vdpDecodeTaskToDecoded
	// VideoDecoderPipeline::OnFrameDecoded - VideoDecoderPipeline::OnFrameProcessed
	vdpDecodedToProcessed
	// VideoDecoderPipeline::OnFrameProcessed - VideoDecoderPipeline::OnFrameConverted
	vdpProcessedToConverted
	// VideoDecoderPipeline::OnFrameDecoded - VideoDecoderPipeline::OnFrameConverted
	vdpDecodedToConverted
	// VideoDecoderPipeline::OnFrameConverted - MojoVideoDecoder::OnVideoFrameDecoded
	vdpConvertedToMojoVD

	// decode duration
	// MojoDecoderBufferWriter::Write
	mojoDecoderBufferWrite
	// MojoMojoDecoderBufferReader::Read
	mojoDecoderBufferRead

	maxChromeTraceEventResults
)

// recordDurationOfEvents fills the performance values about the duration of events.
func recordDurationOfEvents(events []chromeEventInfo, resolutions []graphics.Size,
	results *[maxChromeTraceEventResults]map[string][]int64) error {
	durationEventSet := map[string]chromeTraceEventResultIndex{
		"RTCVideoEncoder::Impl::EncodeOneFrame":                rtcEncodeFuncDuration,
		"RTCVideoEncoder::Impl::EncodeOneFrameWithNativeInput": rtcEncodeFuncDuration,
		"PlatformEncoding.Encode":                              veaEncodingDuration,
		"MojoVEAService::EncodingFrameDuration":                gpuEncodingDuration,
		"MojoDecoderBufferWriter::Write":                       mojoDecoderBufferWrite,
		"MojoDecoderBufferReader::Read":                        mojoDecoderBufferRead,
		"VAVEA::ImageProcessor::BlitSurface":                   imageProcessDuration,
		"V4L2VEA::ImageProcessor::Process":                     imageProcessDuration,
	}

	for _, event := range events {
		index, found := durationEventSet[event.name]
		if !found {
			continue
		}

		if event.duration == 0 {
			// This is the case that the tracing ends in the middle of the event.
			continue
		}
		// The duration of PlatformEncoding.Encode, V4L2VEA::ImageProcessor::Process and VAVEA::ImageProcessor::BlitSurface.
		// is dependent on the encoder resolution. We should record them per resolution.
		var argKey string
		switch event.name {
		case "PlatformEncoding.Encode":
			argKey = "size"
		case "V4L2VEA::ImageProcessor::Process":
			argKey = "output_size"
		case "VAVEA::ImageProcessor::BlitSurface":
			argKey = "dest_visible_rect"
		}
		suffix := noPerfNameSuffix
		if argKey != "" {
			value, found := event.strArgs[argKey]
			if !found {
				// The key argument is not recorded when the tracing terminates
				// between TRACE_EVENT_NESTABLE_ASYNC_BEGIN and TRACE_EVENT_NESTABLE_ASYNC_END.
				continue
			}
			var wh []string
			switch event.name {
			case "PlatformEncoding.Encode", "V4L2VEA::ImageProcessor::Process":
				// gfx::Size
				// 1280x720
				wh = strings.Split(value, "x")
				if len(wh) != 2 {
					return errors.Errorf("failed parsing gfx::Size: %s => %v", value, wh)
				}
			case "VAVEA::ImageProcessor::BlitSurface":
				// gfx::Rect
				// 0,0 1280x720
				xyres := strings.Split(value, " ")
				if len(xyres) != 2 {
					return errors.Errorf("failed parsing gfx::Rect: %s => %v", value, xyres)
				}
				wh = strings.Split(xyres[1], "x")
				if len(wh) != 2 {
					return errors.Errorf("failed parsing gfx::Size: %s => %v", xyres[1], wh)
				}
			}

			w, err := strconv.Atoi(wh[0])
			if err != nil {
				return errors.Wrapf(err, "failed converting to integer: %v", wh)
			}
			h, err := strconv.Atoi(wh[1])
			if err != nil {
				return errors.Wrapf(err, "failed converting to integer: %v", wh)
			}
			expectedSize := false
			for _, size := range resolutions {
				if h == size.Height {
					expectedSize = true
					suffix = "h_" + wh[1]
					break
				} else if w == size.Width {
					expectedSize = true
					suffix = "w_" + wh[0]
					break
				}
			}
			if !expectedSize {
				// RTCPeerConnection needs to ramp up the encoded resolution
				// before it reaches the expected one.
				continue
			}
		}
		results[index][suffix] = append(results[index][suffix], event.duration)
	}
	return nil
}

// recordIntervalTimeOfEvents records the performance values about the interval times between events.
func recordIntervalTimeOfEvents(ctx context.Context,
	events []chromeEventInfo,
	results *[maxChromeTraceEventResults]map[string][]int64) error {
	type intervalMap struct {
		from  string
		to    string
		index chromeTraceEventResultIndex
	}
	eventsIntervalMap := []intervalMap{
		{"RTCVideoEncoder::Encode", "MojoVideoEncodeAccelerator::Encode", rtcToMojoVEA},
		{"MojoVideoEncodeAccelerator::Encode", "MojoVideoEncodeAcceleratorService::Encode", mojoVEAToMojoVEAService},
		{"MojoVideoEncodeAcceleratorService::BitstreamBufferReady", "RTCVideoEncoder::Impl::BitstreamBufferReady", mojoVEAServiceToRTC},
		{"MojoVideoDecoder::Decode", "VideoDecoderPipeline::Decode", mojoVDToVDPDecode},
		{"VideoDecoderPipeline::Decode", "VideoDecoderPipeline::DecodeTask", vdpDecodeToDecodeTask},
		{"VideoDecoderPipeline::DecodeTask", "VideoDecoderPipeline::OnFrameDecoded", vdpDecodeTaskToDecoded},
		{"VideoDecoderPipeline::OnFrameDecoded", "VideoDecoderPipeline::OnFrameProcessed", vdpDecodedToProcessed},
		{"VideoDecoderPipeline::OnFrameProcessed", "VideoDecoderPipeline::OnFrameConverted", vdpProcessedToConverted},
		{"VideoDecoderPipeline::OnFrameDecoded", "VideoDecoderPipeline::OnFrameConverted", vdpDecodedToConverted},
		{"VideoDecoderPipeline::OnFrameConverted", "MojoVideoDecoder::OnVideoFrameDecoded", vdpConvertedToMojoVD},
	}
	if arch := runtime.GOARCH; arch == "arm" || arch == "arm64" {
		eventsIntervalMap = append(eventsIntervalMap,
			[]intervalMap{{"MojoVideoEncodeAcceleratorService::Encode", "V4L2VEA::EncodeTask", mojoVEAServiceToVEA},
				{"V4L2VEA::BitstreamBufferReady", "MojoVideoEncodeAcceleratorService::BitstreamBufferReady", veaToMojoVEAService}}...)
	} else {
		eventsIntervalMap = append(eventsIntervalMap,
			[]intervalMap{{"MojoVideoEncodeAcceleratorService::Encode", "VAVEA::EncodeTask", mojoVEAServiceToVEA},
				{"VAVEA::BitstreamBufferReady", "MojoVideoEncodeAcceleratorService::BitstreamBufferReady", veaToMojoVEAService}}...)
	}

	// eventStack stores the event that will be referenced as "from" by a "to" event later.
	// In most cases, eventStack[event name][key timestamp] is sufficient to look up the "from" event.
	// But it is slice because BitstreamBufferReady() is called multiple times in VP9 k-SVC encoding.
	eventStack := make(map[string]map[int64][]chromeEventInfo)
	for _, eventName := range eventsIntervalMap {
		eventStack[eventName.from] = make(map[int64][]chromeEventInfo)
	}
	for _, event := range events {
		name := event.name
		addEventStack := false
		intervalIndex := -1
		for i := 0; i < len(eventsIntervalMap); i++ {
			if eventsIntervalMap[i].from == name {
				addEventStack = true
			}
			if eventsIntervalMap[i].to == name {
				intervalIndex = i
			}
		}
		// This is the duration event.
		if !addEventStack && intervalIndex == -1 {
			continue
		}

		keyTS := event.keyTS
		if addEventStack {
			eventStack[name][keyTS] = append(eventStack[name][keyTS], event)
		}

		if intervalIndex != -1 {
			fromName := eventsIntervalMap[intervalIndex].from
			var fromEvent chromeEventInfo
			found := false
			switch name {
			// In VP9 k-SVC encoding, we need to look up the corresponding BitstreamBufferReady event with "bitstream_buffer_id".
			case "MojoVideoEncodeAcceleratorService::BitstreamBufferReady", "RTCVideoEncoder::Impl::BitstreamBufferReady":
				fromEvents, foundEvents := eventStack[fromName][keyTS]
				if foundEvents {
					for i := 0; i < len(fromEvents); i++ {
						if fromEvents[i].intArgs["bitstream_buffer_id"] == event.intArgs["bitstream_buffer_id"] {
							fromEvent = eventStack[fromName][keyTS][i]
							found = true
							if len(fromEvents) == 1 {
								delete(eventStack[fromName], keyTS)
							} else {
								eventStack[fromName][keyTS] =
									append(eventStack[fromName][keyTS][:i],
										eventStack[fromName][keyTS][i+1:]...)
							}
							break
						}
					}
				}
			default:
				_, found = eventStack[fromName][keyTS]
				if found {
					fromEvent = eventStack[fromName][keyTS][0]
					delete(eventStack[fromName], keyTS)
				}
			}

			if !found {
				// This is possible because tracing starts and ends during encoding and decoding.
				testing.ContextLog(ctx, "skip: ", event)
				continue
			}
			fromTS := fromEvent.eventTS
			resultIndex := eventsIntervalMap[intervalIndex].index
			intervalTime := event.eventTS - fromTS
			results[resultIndex][noPerfNameSuffix] = append(results[resultIndex][noPerfNameSuffix], intervalTime)
		}
	}
	return nil
}

// processTraceOutput returns the performance values from the tracing output data.
func processTraceOutput(ctx context.Context, traceOut [][]string, resolutions []graphics.Size) (
	results [maxChromeTraceEventResults]map[string][]int64, err error) {

	events, err := formatTraceOutput(traceOut)
	if err != nil {
		return results, errors.Wrap(err, "failed in parsing trace results")
	}

	for i := 0; i < len(results); i++ {
		results[i] = make(map[string][]int64)
	}

	if err = recordDurationOfEvents(events, resolutions, &results); err != nil {
		return results, err
	}
	if err = recordIntervalTimeOfEvents(ctx, events, &results); err != nil {
		return results, err
	}

	return results, nil
}

// measureChromeTraceEvents profiles the chrome hardware video decoding and encoding
// for a webrtc peerconnection call using perfetto.
func measureChromeTraceEvents(ctx context.Context, s *testing.State, resolutions []graphics.Size, p *perf.Values) error {
	traceOut, err := recordTracing(ctx, s)
	if err != nil {
		return err
	}

	results, err := processTraceOutput(ctx, traceOut, resolutions)
	if err != nil {
		return err
	}

	const (
		decPrefix    = "Chrome_Decoding"
		decDurPrefix = "Chrome_Decoding_Z_Duration"
		encPrefix    = "Chrome_Encoding"
		encDurPrefix = "Chrome_Encoding_Z_Duration"
	)
	perfNamePrefixes := [maxChromeTraceEventResults]string{
		encPrefix + "_A_" + "RTCVEA_To_MojoVEA",
		encPrefix + "_B_" + "MojoVEA_To_MojoVEAService",
		encPrefix + "_C_" + "MojoVEAService_To_VEA",
		encPrefix + "_D_" + "VEA_To_MojoVEAService",
		encPrefix + "_E_" + "MojoVEAService_To_RTCVEA",

		encDurPrefix + "_A_" + "RTCVEAEncodeFrameCall",
		encDurPrefix + "_B_" + "PlatformEncoding",
		encDurPrefix + "_C_" + "EncodingInGpuProcess",
		encDurPrefix + "_D_" + "ImageProcessing",

		decPrefix + "_A_" + "MojoVD_To_VDP_Decode",
		decPrefix + "_B_" + "VDP_Decode_To_DecodeTask",
		decPrefix + "_C_" + "VDP_DecodeTask_To_Decoded",
		decPrefix + "_D_" + "VDP_Decoded_To_Processed",
		decPrefix + "_E_" + "VDP_Processed_To_Converted",
		decPrefix + "_F_" + "VDP_Decoded_To_Converted",
		decPrefix + "_G_" + "VDP_Converted_To_MojoVD",

		decDurPrefix + "_A_" + "MojoDecoderBufferWrite",
		decDurPrefix + "_B_" + "MojoDecoderBufferRead",
	}

	for i, resultsMap := range results {
		if len(resultsMap) == 0 {
			testing.ContextLog(ctx, "No data about ", perfNamePrefixes[i])
			continue
		}
		for suffix, durs := range resultsMap {
			// The trace event reocrds in nano seconds. We report them in microseconds.
			for j, dur := range durs {
				durs[j] = (time.Duration(dur) * time.Nanosecond).Microseconds()
			}

			perfName := perfNamePrefixes[i]
			if suffix != noPerfNameSuffix {
				perfName += "_" + suffix
			}
			// This is not reported but output the log for the info.
			avg, variance, min, max := computeStats(durs)
			testing.ContextLogf(ctx, "%s (from %d samples): avg=%.2f [us], min=%d [us], max=%d [us], var=%.3f",
				perfName, len(durs), avg, min, max, variance)

			// The interval time and duration usually have large variance. We report the
			// average value of the data between the first and the third quartile points.
			sort.Slice(durs, func(x, y int) bool { return durs[x] < durs[y] })
			durs = durs[len(durs)/4 : len(durs)*3/4]
			if len(durs) > 0 {
				avg, variance, min, max = computeStats(durs)
				testing.ContextLogf(ctx, "%s (from %d samples): avg=%.2f [us], min=%d [us], max=%d [us], var=%.3f",
					perfName, len(durs), avg, min, max, variance)
			}

			p.Set(perf.Metric{
				Name:      perfName,
				Unit:      "us",
				Direction: perf.SmallerIsBetter,
			}, avg)
		}
	}
	return nil
}

// computeStats returns the mean value, the Bessel corrected variance, min and max value from values.
func computeStats(values []int64) (mean, variance float64, min, max int64) {
	min = values[0]
	max = values[0]
	for _, v := range values {
		mean += float64(v)
		if min > v {
			min = v
		}
		if max < v {
			max = v
		}
	}

	mean /= float64(len(values))
	for _, v := range values {
		variance += (float64(v) - mean) * (float64(v) - mean)
	}
	variance /= float64(len(values) - 1) // Bessel's correction.
	return mean, variance, min, max
}
