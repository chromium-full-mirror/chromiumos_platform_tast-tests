// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
// Webcam need support MMAP method & Motion-JPEG format
package utils

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"

	"github.com/blackjack/webcam"

	"go.chromium.org/chromiumos/config/go/test/lab/api/passport"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type pixel = passport.Pixel

var (
	// key:id , value:dev id
	webcamOnline = make(map[int]string)

	// max webcam length
	maxWebcamLen = 20

	// dht
	dhtMarker = []byte{255, 196}
	dht       = []byte{1, 162, 0, 0, 1, 5, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
		1, 0, 3, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 16, 0, 2, 1, 3, 3, 2,
		4, 3, 5, 5, 4, 4, 0, 0, 1, 125, 1, 2, 3, 0, 4, 17, 5, 18, 33, 49, 65, 6, 19, 81, 97, 7, 34, 113, 20, 50, 129,
		145, 161, 8, 35, 66, 177, 193, 21, 82, 209, 240, 36, 51, 98, 114, 130, 9, 10, 22, 23, 24, 25, 26, 37, 38, 39,
		40, 41, 42, 52, 53, 54, 55, 56, 57, 58, 67, 68, 69, 70, 71, 72, 73, 74, 83, 84, 85, 86, 87, 88, 89, 90, 99,
		100, 101, 102, 103, 104, 105, 106, 115, 116, 117, 118, 119, 120, 121, 122, 131, 132, 133, 134, 135, 136,
		137, 138, 146, 147, 148, 149, 150, 151, 152, 153, 154, 162, 163, 164, 165, 166, 167, 168, 169, 170, 178,
		179, 180, 181, 182, 183, 184, 185, 186, 194, 195, 196, 197, 198, 199, 200, 201, 202, 210, 211, 212, 213,
		214, 215, 216, 217, 218, 225, 226, 227, 228, 229, 230, 231, 232, 233, 234, 241, 242, 243, 244, 245, 246,
		247, 248, 249, 250, 17, 0, 2, 1, 2, 4, 4, 3, 4, 7, 5, 4, 4, 0, 1, 2, 119, 0, 1, 2, 3, 17, 4, 5, 33, 49,
		6, 18, 65, 81, 7, 97, 113, 19, 34, 50, 129, 8, 20, 66, 145, 161, 177, 193, 9, 35, 51, 82, 240, 21, 98,
		114, 209, 10, 22, 36, 52, 225, 37, 241, 23, 24, 25, 26, 38, 39, 40, 41, 42, 53, 54, 55, 56, 57, 58, 67,
		68, 69, 70, 71, 72, 73, 74, 83, 84, 85, 86, 87, 88, 89, 90, 99, 100, 101, 102, 103, 104, 105, 106, 115,
		116, 117, 118, 119, 120, 121, 122, 130, 131, 132, 133, 134, 135, 136, 137, 138, 146, 147, 148, 149, 150,
		151, 152, 153, 154, 162, 163, 164, 165, 166, 167, 168, 169, 170, 178, 179, 180, 181, 182, 183, 184, 185,
		186, 194, 195, 196, 197, 198, 199, 200, 201, 202, 210, 211, 212, 213, 214, 215, 216, 217, 218, 226, 227,
		228, 229, 230, 231, 232, 233, 234, 242, 243, 244, 245, 246, 247, 248, 249, 250}
	sosMarker = []byte{255, 218}
)

// InitWebcam initializes Webcam.
func InitWebcam(ctx context.Context) error {
	var cams = make(map[string](*webcam.Webcam))
	for i := 0; i < maxWebcamLen; i++ {
		s := fmt.Sprintf("/dev/video%d", i)
		cam, err := webcam.Open(s) // Open webcam
		if err != nil {
			continue
		}
		err = cam.StartStreaming()
		if err != nil {
			continue
		}
		formatDesc := cam.GetSupportedFormats()
		isSupportMotionJPEG := false
		for f := range formatDesc {
			if formatDesc[f] == "Motion-JPEG" {
				isSupportMotionJPEG = true
				break
			}
		}

		if isSupportMotionJPEG {
			cams[s] = cam
			cam.Close()
		} else {
			testing.ContextLogf(ctx, "%s didn't support Motion-JPEG format", s)
		}
	}

	webcamsCount := 0
	for video, cam := range cams {
		name, err := cam.GetName()
		if err != nil {
			name = err.Error()
		}
		testing.ContextLogf(ctx, "online cam: %s, %s", video, name)
		webcamOnline[webcamsCount] = video
		webcamsCount = webcamsCount + 1
	}

	return nil
}

// CamerasOnline fetches the found cameras.
func CamerasOnline() []string {
	var cameras []string
	for _, value := range webcamOnline {
		cameras = append(cameras, value)
	}
	return cameras
}

// addMotionDht is for add header to JPEG file.
func addMotionDht(frame []byte) []byte {
	jpegParts := bytes.Split(frame, sosMarker)
	return append(jpegParts[0], append(dhtMarker, append(dht, append(sosMarker, jpegParts[1]...)...)...)...)
}

// GetAvgPixelFromWebcam is for get avg pixel from webcam.
func GetAvgPixelFromWebcam(ctx context.Context, devPort string) (*pixel, []byte, error) {
	var p *pixel
	cam, err := webcam.Open(devPort)

	if err != nil {
		return nil, nil, errors.New(devPort + " not found")
	}
	defer cam.Close()

	formatDesc := cam.GetSupportedFormats()
	for f := range formatDesc {
		if formatDesc[f] == "Motion-JPEG" {
			format := f
			_, _, _, err := cam.SetImageFormat(format, uint32(600), uint32(600))

			if err != nil {
				return nil, nil, errors.New("Set image format failed")
			}

			break
		}
	}

	err = cam.StartStreaming()
	if err != nil {
		return nil, nil, errors.Wrap(err, devPort+" streaming failed")
	}

	// 5 seconds time out.
	frameCount := 0
	timeout := uint32(5)
	for {
		err = cam.WaitForFrame(timeout)

		switch err.(type) {
		case nil:
		case *webcam.Timeout:
			fmt.Fprint(os.Stderr, err.Error())
			continue
		default:
			return nil, nil, errors.New(devPort + " webcam take frame time out failed")
		}

		frame, err := cam.ReadFrame()
		frameCount++

		if err != nil {
			return nil, nil, err
		} else if frameCount > 10 && len(frame) != 0 {
			frame = addMotionDht(frame)

			if err != nil {
				return nil, nil, errors.New("get Pixel From Webcam write file error: " + devPort)
			}

			image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig)
			p, err = getAvgPixelColor(bytes.NewReader(frame))
			if err != nil {
				return nil, nil, errors.New("get Pixel From Webcam error: " + devPort)
			}

			return p, frame, nil
		}
	}
}

// getAvgPixelColor is for get the bi-dimensional pixel array.
func getAvgPixelColor(file io.Reader) (*pixel, error) {
	img, _, err := image.Decode(file)

	if err != nil {
		fmt.Println(err.Error())
		return nil, err
	}

	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	var pixelsCount = 0
	var redSum float64
	var greenSum float64
	var blueSum float64
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixelXY := rgbaToPixel(img.At(x, y).RGBA())
			redSum += float64(pixelXY.R)
			greenSum += float64(pixelXY.G)
			blueSum += float64(pixelXY.B)
			pixelsCount++
		}
	}

	p := &pixel{
		R: int32(redSum / float64(pixelsCount)),
		G: int32(greenSum / float64(pixelsCount)),
		B: int32(blueSum / float64(pixelsCount)),
		A: 255}

	return p, nil
}

// rgbaToPixel is for translate from rgb value to Pixel format.
func rgbaToPixel(r, g, b, a uint32) pixel {
	return pixel{
		R: int32(r / 257),
		G: int32(g / 257),
		B: int32(b / 257),
		A: int32(a / 257)}
}
