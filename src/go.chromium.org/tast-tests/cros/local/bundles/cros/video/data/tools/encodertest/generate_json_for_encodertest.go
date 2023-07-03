// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Tool for generating a json file used in video_encode_accelerator_tests for
// the vp9 webm file. The json file is created in the same directory as the video file.
// This script uses go.chromium.org/tast/core/errors, so it needs to run with
// ~/trunk/src/platform/tast/tools/go.sh.
// This also computes the md5 value of the first 60 frames of the vp9 webm in I420 format.
// It is useful to fill the table to verify vpxdec in the test is correct. See
// https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/video/encode/accel_video.go
//
// Usage example:
// $  ~/trunk/src/platform/tast/tools/go.sh run generate_json_for_encodertest.go desktop2-1280x720_850frames.vp9.webm
package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// getNumFrames gets the number of video frames in the vp9 webm file.
func getNumFrames(file string) (int, error) {
	out, err := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json",
		"-select_streams", "v", "-show_frames", file).Output()
	if err != nil {
		return 0, errors.Wrap(err, "failed executing ffprobe")
	}
	type ffprobeFramesJSON struct {
		Frames []struct {
		} `json:"frames"`
	}
	var jsonData ffprobeFramesJSON
	if err = json.Unmarshal(out, &jsonData); err != nil {
		return 0, errors.Wrap(err, "failed unmarshaling json file")
	}

	if len(jsonData.Frames) == 0 {
		return 0, errors.Errorf("ffprobe fails to count frames: %+v", jsonData.Frames)
	}

	return len(jsonData.Frames), nil
}

// jsonInfo stores the info in a json file used in video_encode_accelerator_tests.
type jsonInfo struct {
	Profile   string `json:"profile"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	NumFrames int    `json:"num_frames"`
	FrameRate int    `json:"frame_rate"`
}

// parseVP9WebMStream gets the width, height, framerate in the vp9 webm file.
func parseVP9WebMStream(file string) (width, height, frameRate int, err error) {
	out, err := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json",
		"-select_streams", "v", "-show_streams", file).Output()
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "failed executing ffprobe")
	}

	type ffprobeJSON struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
			Profile   string `json:"profile"`
			PixFmt    string `json:"pix_fmt"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			FrameRate string `json:"r_frame_rate"`
		} `json:"streams"`
	}

	var jsonData ffprobeJSON
	if err = json.Unmarshal(out, &jsonData); err != nil {
		return 0, 0, 0, errors.Wrap(err, "failed unmarshaling json file")
	}

	if len(jsonData.Streams) != 1 {
		return 0, 0, 0, errors.Errorf("ffprobe detects multiple or no video streams: %+v", jsonData.Streams)
	}

	stream := jsonData.Streams[0]
	if stream.CodecName != "vp9" || stream.Profile != "Profile 0" {
		return 0, 0, 0, errors.Errorf("unexpected input codec: %+v", stream)
	}

	if stream.PixFmt != "yuv420p" {
		return 0, 0, 0, errors.Errorf("unexpected input pixel format: %+v", stream)
	}

	fpsFrac := strings.Split(stream.FrameRate, "/")
	if len(fpsFrac) != 2 {
		return 0, 0, 0, errors.Errorf("invalid framerate=%s", stream.FrameRate)
	}

	fpsNum, err := strconv.ParseFloat(fpsFrac[0], 64)
	if err != nil {
		return 0, 0, 0, err
	}

	fpsDenom, err := strconv.ParseFloat(fpsFrac[1], 64)
	if err != nil {
		return 0, 0, 0, err
	} else if fpsDenom == 0 {
		return 0, 0, 0, errors.New("fps denomination is zero")
	}
	return stream.Width, stream.Height, int(math.Round(fpsNum / fpsDenom)), nil
}

// createJSONInfo creates the jsonInfo for the vp9 webm file.
func createJSONInfo(file string) (*jsonInfo, error) {
	var info jsonInfo
	var err error
	info.Width, info.Height, info.FrameRate, err = parseVP9WebMStream(file)
	if err != nil {
		return nil, errors.Wrap(err, "failed parsing stream")
	}

	info.NumFrames, err = getNumFrames(file)
	if err != nil {
		return nil, errors.Wrap(err, "failed generating frame hashes")
	}

	info.Profile = "VP9PROFILE_PROFILE0"
	return &info, nil
}

// writeJSONFor writes jsonInfo to the json file corresponding to the file.
// The created json file path is returned.
func (info *jsonInfo) writeJSONFor(file string) (string, error) {
	s, err := json.MarshalIndent(info, "", "\t")
	if err != nil {
		return "", errors.Wrap(err, "failed marshaling json")
	}

	jsonPath := file + ".json"
	return jsonPath, ioutil.WriteFile(jsonPath, s, 0644)
}

// computeMD5SumOf60Frames computes the md5 of YUV file of the first 60 frames
// in the vp9 file.
func computeMD5SumOf60Frames(file string) (string, error) {
	command := []string{"vpxdec", file, "-o", "-", "--codec=vp9", "--i420", "--limit=60"}
	cmd := exec.Command(command[0], command[1:]...)
	yuv, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(err, "vpxdec failed")
	}

	if len(yuv) >= (1 << 30) {
		return "", errors.Errorf("the size of YUV frames >= 1 GiB, %d bytes", len(yuv))
	}
	md5sum := md5.Sum(yuv)
	return hex.EncodeToString(md5sum[:]), nil
}

func main() {
	for _, file := range os.Args[1:] {
		ext := filepath.Ext(file)
		if ext != ".webm" {
			log.Fatalf("The file's extension must be vp9.webm: %s\n", file)
		}

		info, err := createJSONInfo(file)
		if err != nil {
			log.Fatalf("Failed getting json data for %s: %v\n", file, err)
		}

		jsonFile, err := info.writeJSONFor(file)
		if err != nil {
			log.Fatalf("Failed writing json data for %s: %v\n", file, err)
		}

		fmt.Println("Created", jsonFile)
		// Output the md5sum of the first 60 frames. This can be used to add the entry to encode/accel_video.go.
		md5sum, err := computeMD5SumOf60Frames(file)
		if err != nil {
			log.Fatalf("Failed computing md5 values of the 60 frames %s: %v\n", file, err)
		}
		fmt.Printf("\"%s\": \"%s\",\n", file, md5sum)
	}
}
