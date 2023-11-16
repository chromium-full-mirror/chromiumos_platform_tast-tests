// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cca

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abema/go-mp4"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing/hwdep"
)

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

// videoInfo returns information of a video by ffprobe. See https://ffmpeg.org/ffprobe.html#Main-options for the definition of entries.
func videoInfo(ctx context.Context, entry, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open file %v", path)
	}
	defer f.Close()
	args := []string{"-v", "error", "-select_streams", "v", "-show_entries", entry, "-of", "default=nw=1:nk=1", path}
	output, err := testexec.CommandContext(ctx, "ffprobe", args...).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to run ffprobe")
	}
	return strings.TrimSpace(string(output)), nil
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
