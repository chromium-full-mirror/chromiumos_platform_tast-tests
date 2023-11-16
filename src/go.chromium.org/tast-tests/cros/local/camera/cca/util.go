// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cca

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abema/go-mp4"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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

// VideoDuration returns duration of the video file in the given |path|.
func VideoDuration(ctx context.Context, path string) (time.Duration, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to open file %v", path)
	}
	defer f.Close()

	fraInfo, err := mp4.ProbeFra(f)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to probe fragments from %v", path)
	}

	duration := 0.0
	if len(fraInfo.Segments) == 0 {
		// Regular MP4
		boxes, err := mp4.ExtractBoxWithPayload(f, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeMvhd()})
		if err != nil {
			return 0, errors.Wrapf(err, "failed to parse mp4 header from %v", path)
		}
		if len(boxes) == 0 {
			return 0, errors.New("no mvhd box found")
		}
		mvhd, ok := boxes[0].Payload.(*mp4.Mvhd)
		if !ok {
			return 0, errors.New("got invalid mvhd box")
		}
		duration = float64(mvhd.DurationV0) / float64(mvhd.Timescale)
		// TODO(crbug.com/1140852): Remove the logging once we fully migrated to regular mp4.
		testing.ContextLogf(ctx, "Found a regular mp4 with duration %.2fs", duration)
	} else {
		// Fragmented MP4
		// TODO(crbug.com/1140852): Remove fmp4 code path once we fully migrated to regular mp4.
		for _, s := range fraInfo.Segments {
			duration += float64(s.Duration) / float64(fraInfo.Tracks[s.TrackID-1].Timescale)
		}
		testing.ContextLogf(ctx, "Found a fragmented mp4 with duration %.2fs", duration)
	}

	return time.Duration(duration * float64(time.Second)), nil
}

// CheckVideoMuted returns error if the check fails or the video file in the given |path| is not muted.
func CheckVideoMuted(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %v", path)
	}
	defer file.Close()

	args := []string{"-i", path, "-filter:a", "volumedetect", "-f", "null", "-"}
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
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
