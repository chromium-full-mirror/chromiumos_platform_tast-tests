// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cca

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abema/go-mp4"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
)

type actionFunc func(ctx context.Context) error

// DeviceWithLayoutMonitored lists the devices we want to monitor the layout correctness.
var DeviceWithLayoutMonitored = hwdep.D(hwdep.Model(
	"atlas",
	"betty",
	"eve",
	"nocturne",
	"soraka",
))

// CheckVideoProfile checks profile of video file recorded by CCA.
func CheckVideoProfile(path string, profile Profile) error {
	videoAVCConfigure := func(path string) (*mp4.AVCDecoderConfiguration, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open video file %v", path)
		}
		defer file.Close()
		boxes, err := mp4.ExtractBoxWithPayload(
			file, nil,
			mp4.BoxPath{
				mp4.BoxTypeMoov(),
				mp4.BoxTypeTrak(),
				mp4.BoxTypeMdia(),
				mp4.BoxTypeMinf(),
				mp4.BoxTypeStbl(),
				mp4.BoxTypeStsd(),
				mp4.StrToBoxType("avc1"),
				mp4.StrToBoxType("avcC"),
			})
		if err != nil {
			return nil, err
		}
		if len(boxes) != 1 {
			return nil, errors.Errorf("mp4 file %v has %v avcC box(es), want 1", path, len(boxes))
		}
		return boxes[0].Payload.(*mp4.AVCDecoderConfiguration), nil
	}

	config, err := videoAVCConfigure(path)
	if err != nil {
		return errors.Wrap(err, "failed to get videoAVCConfigure from result video")
	}
	if int(config.Profile) != int(profile.Value) {
		return errors.Errorf("mismatch video profile, got %v; want %v", config.Profile, profile.Value)
	}
	return nil
}

func parseResolution(s string) (*Resolution, error) {
	fs := strings.Fields(s)

	if len(fs) != 2 {
		return nil, errors.Errorf("expect exactly 2 space separated numbers, found %d", len(fs))
	}

	w, err := strconv.Atoi(fs[0])
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse width")
	}

	h, err := strconv.Atoi(fs[1])
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse height")
	}

	return &Resolution{Width: w, Height: h}, nil
}

// PhotoResolution returns the resolution of a jpeg photo file.
func PhotoResolution(ctx context.Context, path string) (*Resolution, error) {
	args := []string{
		"-auto-orient", // Handles orientation tag in jpeg EXIF.
		"-format",
		"%w %h",
		path,
		"info:",
	}
	out, err := testexec.CommandContext(ctx, "convert", args...).Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run convert")
	}
	return parseResolution(string(out))
}

// VideoResolution returns the resolution of a mp4 video file.
func VideoResolution(ctx context.Context, path string) (*Resolution, error) {
	output, err := videoInfo(ctx, "stream=width,height", path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get video info")
	}

	res, err := parseResolution(output)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse resolution")
	}

	// The video may have rotation metadata and it should be applied properly.
	rot := 0
	output, err = videoInfo(ctx, "stream_side_data=rotation", path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get video info")
	}
	if output != "" {
		rot, err = strconv.Atoi(output)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse rotation")
		}
	}

	rot = (rot + 360) % 360
	if rot == 90 || rot == 270 {
		return &Resolution{Width: res.Height, Height: res.Width}, nil
	}
	return res, nil
}

// ExtractResolution extracts the resolution of given photo or video file.
func ExtractResolution(ctx context.Context, path string) (*Resolution, error) {
	switch ext := filepath.Ext(path); ext {
	case ".mp4":
		return VideoResolution(ctx, path)
	case ".jpg":
		return PhotoResolution(ctx, path)
	default:
		return nil, errors.Errorf("unexpected extension name %s", ext)
	}
}

// VideoDurationFromHeader returns the duration of the video file in the given "path", extracted from the header.
func VideoDurationFromHeader(ctx context.Context, path string) (time.Duration, error) {
	output, err := videoInfo(ctx, "format=duration", path)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get video info")
	}
	seconds, err := strconv.ParseFloat(output, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse duration output as a float")
	}
	duration := time.Duration(seconds * float64(time.Second))
	return duration, nil
}

// BaseVideoFPS returns the base frame rate of the video, guessed from the video's time base and each frame's timer ticks (see https://github.com/FFmpeg/FFmpeg/blob/release/5.1/libavformat/avformat.h#L1099).
func BaseVideoFPS(ctx context.Context, path string) (float64, error) {
	output, err := videoInfo(ctx, "stream=r_frame_rate", path)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get info of base video FPS")
	}
	return parseFraction(output)
}

func parseFraction(fraction string) (float64, error) {
	parts := strings.Split(fraction, "/")
	if len(parts) != 2 {
		return 0, errors.New("invalid faction format")
	}

	numerator, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse numerator")
	}

	denominator, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse denominator")
	}

	return numerator / denominator, nil
}

// videoInfo returns information of a video by ffprobe. See https://ffmpeg.org/ffprobe.html#Main-options for the definition of entries.
func videoInfo(ctx context.Context, entry, path string, extraArgs ...string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open file %v", path)
	}
	defer f.Close()
	args := []string{"-v", "error", "-select_streams", "v", "-show_entries", entry, "-of", "default=nw=1:nk=1"}
	args = append(args, extraArgs...)
	args = append(args, path)
	output, err := testexec.CommandContext(ctx, "ffprobe", args...).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to run ffprobe")
	}
	return strings.TrimSpace(string(output)), nil
}

// NumberOfKeyFrames returns a number of key frames in a video in the specified |path|.
func NumberOfKeyFrames(ctx context.Context, path string) (int, error) {
	// Skip printing information of non-key frames to ignore unnecessary information in the output.
	skipNonKeyFrameArgs := []string{"-skip_frame", "nokey"}
	output, err := videoInfo(ctx, "frame=key_frame", path, skipNonKeyFrameArgs...)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get video info")
	}
	// The command output is a multi-line string where each line contains the
	// digit 1. Number of key frames equal to the number of lines, or the
	// number of "1".
	return strings.Count(output, "1"), nil
}

// CheckVideoMuted returns error if the check fails or the video file in the given |path| is not muted.
func CheckVideoMuted(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %v", path)
	}
	defer file.Close()

	args := []string{"-i", path, "-filter:a", "volumedetect", "-f", "null", "-"}
	output, err := testexec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to execute ffmpeg command")
	}
	lines := strings.Split(string(output), "\n")
	// Example line: `[Parsed_volumedetect_0 @ 0x584d634e9920] max_volume: -91.0 dB`
	re := regexp.MustCompile(`\[Parsed_volumedetect_[0-9]+ @ 0x[0-9a-f]+\] max_volume: (.*?) dB`)
	for _, line := range lines {
		submatch := re.FindStringSubmatch(line)
		if len(submatch) > 0 {
			// In case of a muted video, the max volume is -91.0 dB and if it isn't, the max volume is bigger than that.
			vol, err := strconv.ParseFloat(submatch[1], 64)
			if err != nil {
				return errors.Wrap(err, "failed to convert float to string")
			} else if vol > -91.0 {
				return errors.Errorf("video is not muted. Expected max volume: -91.0 dB but got %v dB", vol)
			}
			return nil
		}
	}

	return errors.New("volume detection failed")
}

func recordSound(ctx context.Context, recording audio.TestRawData, recordingErr chan error) {
	if err := crastestclient.CaptureFileCommand(
		ctx,
		recording.Path,
		recording.Duration,
		recording.Channels,
		recording.Rate).Run(testexec.DumpLogOnError); err != nil {
		recordingErr <- errors.Wrap(err, "failed to capture sound")
		return
	}
	recordingErr <- nil
}

// VerifySound records the sound by loopback when triggering the given `action`
// and return true if any sound is played.
func VerifySound(ctx context.Context, action actionFunc) (bool, error) {
	duration := 5 * time.Second
	captureFile, err := ioutil.TempFile("", "")
	if err != nil {
		return false, err
	}
	defer os.Remove(captureFile.Name())

	recording := audio.TestRawData{
		Path:          captureFile.Name(),
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
		Duration:      int(duration.Seconds()),
	}

	if ctxutil.DeadlineBefore(ctx, time.Now().Add(duration)) {
		return false, errors.New("failed to start sound recording since the remaining time in this context is not sufficient")
	}

	testing.ContextLogf(ctx, "Capture sound for %v seconds", recording.Duration)
	recordingErr := make(chan error)
	go recordSound(ctx, recording, recordingErr)

	// Wait until the sound recording starts.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := testexec.CommandContext(ctx, "pgrep", "-f", "cras_test_client").Run(); err != nil {
			return errors.Wrap(err, "the sound recording hasn't been started")
		}
		return nil
	}, nil); err != nil {
		return false, errors.Wrap(err, "failed to wait for the sound recording to start")
	}

	if err := action(ctx); err != nil {
		return false, errors.Wrap(err, "failed to run action successfully")
	}

	err = <-recordingErr
	if err != nil {
		return false, errors.Wrap(err, "failed to record sound successfully")
	}

	rms, err := audio.GetRmsAmplitude(ctx, recording)
	if err != nil {
		return false, errors.Wrap(err, "failed to get Rms Amplitude")
	}
	return rms != 0, nil
}
