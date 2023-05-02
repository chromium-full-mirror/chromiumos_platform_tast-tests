// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"chromiumos/tast/common/testexec"
)

// CommandBuilderDecodeFn is the function type to generate the command line with arguments.
type CommandBuilderDecodeFn func(ctx context.Context, filename, md5OutputPath string) (command []string)

// V4L2StatefulDecodeArgs provides the arguments to use with the stateful decoding binary exe for v4l2.
func V4L2StatefulDecodeArgs(ctx context.Context, filename, md5OutputPath string) (command []string) {
	command = append(command, "--file="+filename, "--log_level=1")

	// Query the driver info. If we are on a MediaTek platform, add --mmap to the
	// command line.
	v4l2CtlCmd := testexec.CommandContext(ctx, "v4l2-ctl", "--device",
		"/dev/video-dec0", "-D")
	v4l2Out, err := v4l2CtlCmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return
	}
	mtkDetect := regexp.MustCompile(`mtk-vcodec-dec`)
	if mtkDetect.MatchString(string(v4l2Out)) {
		command = append(command, "--mmap")
	}
	return
}

// V4L2StatelessDecodeArgs provides the arguments to use with the stateless decoding binary exe for v4l2.
func V4L2StatelessDecodeArgs(ctx context.Context, filename, md5OutputPath string) (command []string) {
	// TODO(stevecho): md5 support has to be added
	command = append(command,
		"--video="+filename,
		// vpxdec is used to compute reference hashes, and outputs only those for
		// visible frames
		"--visible")

	// Query the driver info. If we are on a MediaTek platform, add --mmap to the
	// command line.
	v4l2CtlCmd := testexec.CommandContext(ctx, "v4l2-ctl", "--device",
		"/dev/video-dec0", "-D")
	v4l2Out, err := v4l2CtlCmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return
	}
	mtkDetect := regexp.MustCompile(`mtk-vcodec-dec`)
	if mtkDetect.MatchString(string(v4l2Out)) {
		command = append(command, "--mmap")
	}
	return
}

// getVAAPIArgs provides the arguments to use with different decoding binary exes for vaapi.
func getVAAPIArgs(ctx context.Context, filename string) []string {
	return []string{
		"--video=" + filename,
		// aomdec is used to compute reference hashes, and outputs only those for
		// visible frames
		"--visible",
	}
}

// AV1DecodeVAAPIargs provides the arguments to use with the AV1 decoding binary exe for vaapi.
func AV1DecodeVAAPIargs(ctx context.Context, filename, md5OutputPath string) []string {
	return append(getVAAPIArgs(ctx, filename), "--codec=AV1")
}

// VP9DecodeVAAPIargs provides the arguments to use with the VP9 decoding binary exe for vaapi.
func VP9DecodeVAAPIargs(ctx context.Context, filename, md5OutputPath string) []string {
	return append(getVAAPIArgs(ctx, filename), "--codec=VP9")
}

// VP8DecodeVAAPIargs provides the arguments to use with the VP8 decoding binary exe for vaapi.
func VP8DecodeVAAPIargs(ctx context.Context, filename, md5OutputPath string) []string {
	return append(getVAAPIArgs(ctx, filename), "--codec=VP8")
}

// H264DecodeVAAPIargs provides the arguments to use with the H264 decoding binary exe for vaapi.
func H264DecodeVAAPIargs(ctx context.Context, filename, md5OutputPath string) []string {
	return append(getVAAPIArgs(ctx, filename), "--codec=H264")
}

// HEVCDecodeVAAPIargs provides the arguments to use with the HEVC decoding binary exe for vaapi.
func HEVCDecodeVAAPIargs(ctx context.Context, filename, md5OutputPath string) []string {
	return append(getVAAPIArgs(ctx, filename), "--codec=H265")
}

// MD5Arg returns the argument to specify MD5 hash output for each platform decoder supporting it.
func MD5Arg(decoder, md5OutputPath string) string {
	switch {
	case strings.Contains(decoder, "v4l2"):
		return fmt.Sprintf("--md5=%s", md5OutputPath)
	case strings.Contains(decoder, "decode_test"):
		return fmt.Sprintf("--md5=%s", md5OutputPath)
	}
	return ""
}

// LoopArg returns the argument to specify looping decode for each platform decoder supporting it.
func LoopArg(decoder string, numLoops int) string {
	if strings.Contains(decoder, "decode_test") {
		if numLoops > 0 {
			return fmt.Sprintf("--loop=%d", numLoops)
		}
		return "--loop"
	}
	return ""
}

// VPxDecodeArgs provides the arguments to use with vpxdec decoding binary exe.
func VPxDecodeArgs(ctx context.Context, filename, md5OutputPath string) []string {
	// With --md5 and -o options the md5 of each frame is calculated but frame
	// files are not created.
	return []string{"-o", "output%w_%h_%4.yuv", "--i420", "--md5", filename}
}

// Openh264DecodeArgs provides the arguments to use with openh264dec decoding binary exe.
func Openh264DecodeArgs(ctx context.Context, filename, md5OutputPath string) []string {
	return []string{filename, filename + ".yuv"}
}
