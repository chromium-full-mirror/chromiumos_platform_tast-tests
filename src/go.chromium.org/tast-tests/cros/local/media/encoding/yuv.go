// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package encoding

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// md5OfYUV is the MD5 value of the YUV file decoded by vpxdec.
// Since the decoding algorithm is deterministic, the raw data MD5 value should always be the same.
// These values are listed for the safety check to ensure we are always testing the same raw streams for result consistency.
var md5OfYUV = map[string]string{
	"bear-320x192.i420.yuv":              "14c9ac6f98573ab27a7ed28da8a909c0",
	"crowd-1920x1080.i420.yuv":           "96f60dd6ff87ba8b129301a0f36efc58",
	"gipsrestat-1280x720.i420.yuv":       "acc6bb983c198c8db5ffc5d5699cb235",
	"gipsrestat-640x360.i420.yuv":        "a92466c51d8626f263771ba16f7d5d02",
	"gipsrestat-320x180.i420.yuv":        "556d908527aea47e0e02440bf6c35861",
	"tulip2-1280x720.i420.yuv":           "1b95123232922fe0067869c74e19cd09",
	"tulip2-960x540.i420.yuv":            "c0ff5b6c62ba8914aa073d630acd6309",
	"tulip2-640x360.i420.yuv":            "094bd827de18ca196a83cc6442b7b02f",
	"tulip2-480x270.i420.yuv":            "06e0ac8b028a78ed4cd8dda5ab5bceec",
	"tulip2-320x180.i420.yuv":            "55be7124b3aec1b72bfb57f433297193",
	"tulip2-240x135.i420.yuv":            "057e8f55862161dc231bfd882220159f",
	"vidyo1-1280x720.i420.yuv":           "b8601dd181bb2921fffce3fbb896351e",
	"crowd-3840x2160.i420.yuv":           "c0cf5576391ec6e2439a8d0fc7207662",
	"crowd-641x361.i420.yuv":             "124d3e29ea68eaba0dc35243b4dfc27b",
	"crowd-320x180_30frames.i420.yuv":    "795d9e03fc4631245558cc522462a1e5",
	"crowd-480x270_30frames.i420.yuv":    "21c426bea751e475533d2480b4b33426",
	"crowd-640x360_30frames.i420.yuv":    "134fecaaae471820dede6c761e4d8f4b",
	"crowd-960x540_30frames.i420.yuv":    "c1ab2a4af9bc76fc5d659fcf19fbee09",
	"crowd-1280x720_30frames.i420.yuv":   "f26bff398809056165be970922492281",
	"crowd-1920x1080_30frames.i420.yuv":  "13e4f50ad665e27c2a8603d6e65a0a39",
	"crowd-3840x2160_30frames.i420.yuv":  "a739d49d4072bc91ca7b9b223e4b117f",
	"static-1920x1080_30frames.i420.yuv": "f7d07243a9b5bbaa77930e66c9b64379",
}

// DecodeInI420 decodes all the frames in webMFile and saved in an I420 file.
// The returned value is the path of the created I420 file.
func DecodeInI420(ctx context.Context, webMFile string) (string, error) {
	return DecodeInI420WithNumFrames(ctx, webMFile, -1, "")
}

// DecodeInI420WithNumFrames decodes the specified number of frames in webMFile and saved in an I420 file.
// The returned value is the path of the created an I420 file.
// If the number of frames to be decoded is specified, the expected md5 value needs to be specified.
// It must be removed in the end of test, because its size is expected to be large.
// The input WebM file must be vp9 webm file. They are generated from raw YUV data by libvpx like "vpxenc foo.yuv -o foo.webm --codec=vp9 -w <width> -h <height> --lossless=1"
// Please use "--lossless=1" option. Lossless compression is required to ensure we are testing streams at the same quality as original raw streams,
// to test encoder capabilities (performance, bitrate convergence, etc.) correctly and with sufficient complexity/PSNR.
func DecodeInI420WithNumFrames(ctx context.Context, webMFile string, numFrames int, md5 string) (string, error) {
	if numFrames > 0 && md5 == "" {
		return "", errors.New("md5 must be given if the number of frames is specified")
	}
	const webMSuffix = ".vp9.webm"
	if !strings.HasSuffix(webMFile, webMSuffix) {
		return "", errors.Errorf("source video %v must be VP9 WebM", webMFile)
	}
	webMName := filepath.Base(webMFile)
	yuvFile := strings.TrimSuffix(webMFile, ".vp9.webm") + ".i420.yuv"
	yuvName := filepath.Base(yuvFile)

	var expectedHash string
	if md5 != "" {
		expectedHash = md5
	} else {
		expectedHash = md5OfYUV[yuvName]
	}
	// If the raw video file already exists and the hash matches the expected value we can skip extraction.
	if _, err := os.Stat(yuvFile); !os.IsNotExist(err) {
		yuvHash, err := calculateHash(yuvFile)
		if err != nil {
			return "", err
		}

		if yuvHash == expectedHash {
			testing.ContextLogf(ctx, "Skipping extraction of %s: %s already exists", webMName, yuvName)
			return yuvFile, nil
		}
	}

	tf, err := os.Create(yuvFile)
	if err != nil {
		return "", errors.Wrap(err, "failed to create a temporary YUV file")
	}
	keep := false
	defer func() {
		tf.Close()
		if !keep {
			os.Remove(yuvFile)
		}
	}()

	threads := runtime.NumCPU()
	if threads > 16 {
		// The maximum number of threads is the same as chrome.
		// https://source.chromium.org/chromium/chromium/src/+/main:media/base/limits.h;l=83;drc=5539ecff898c79b0771340051d62bf81649e448d
		threads = 16
	}
	command := []string{"vpxdec", webMFile, "-t", strconv.Itoa(threads), "-o", yuvFile, "--codec=vp9", "--i420"}
	if numFrames > 0 {
		command = append(command, "--limit="+strconv.Itoa(numFrames))
	}
	testing.ContextLogf(ctx, "Running %s", shutil.EscapeSlice(command))
	cmd := testexec.CommandContext(ctx, command[0], command[1:]...)
	if err := cmd.Run(); err != nil {
		cmd.DumpLog(ctx)
		return "", errors.Wrap(err, "vpxdec failed")
	}

	// This guarantees that the generated yuv file (i.e. input of VEA test) is the same on all platforms.
	yuvHash, err := calculateHash(yuvFile)
	if err != nil {
		return "", err
	}
	if yuvHash != expectedHash {
		return "", errors.Errorf("unexpected MD5 value of %s (got %s, want %s)", yuvName, yuvHash, md5OfYUV[yuvName])
	}

	keep = true
	return yuvFile, nil
}

// PrepareYUVJSON creates a json file for yuvPath by copying jsonPath.
// The first return value is the path of the created JSON file.
func PrepareYUVJSON(ctx context.Context, yuvPath, jsonPath string) (string, error) {
	yuvJSONPath := yuvPath + ".json"
	if err := fsutil.CopyFile(jsonPath, yuvJSONPath); err != nil {
		return "", errors.Wrapf(err, "failed to copy json file: %v %v", jsonPath, yuvJSONPath)

	}
	return yuvJSONPath, nil
}

// calculateHash calculates the MD5 hash of the specified file.
func calculateHash(filepath string) (string, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return "", errors.Wrap(err, "failed to open YUV file")
	}
	defer f.Close()

	hasher := md5.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", errors.Wrap(err, "failed to read YUV file")
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
