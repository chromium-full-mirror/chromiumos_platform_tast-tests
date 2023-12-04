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
	"math"
	"os"
	"path"
	"time"

	"github.com/blackjack/webcam"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Pixel struct
type Pixel struct {
	R int
	G int
	B int
	A int
}

var (
	redColor   = Pixel{240, 80, 80, 255}   // RGBA
	greenColor = Pixel{80, 240, 80, 255}   // RGBA
	blueColor  = Pixel{80, 80, 240, 255}   // RGBA
	whiteColor = Pixel{250, 250, 250, 255} // RGBA
	grayColor  = Pixel{120, 120, 120, 255} // RGBA

	detectVideoColor = [3]string{"red", "green", "blue"}

	// key:id , value:dev id
	webcamOnline = make(map[int]string)

	// max webcam length
	maxWebcamLen = 20

	// webcam limit score
	webcamMappingDisplayFixtureLimitScore = 40.0

	// webcam limit score
	webcamMappingLimitScore = 160

	// key: screen id (or 'dut' & fixture uid) , value: webcam port
	screenToCamera = make(map[string]string)

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

// DUTMonitor is the definition word of DUT.
var DUTMonitor = "dut"

// RedPicFileName is the red pic file name.
var RedPicFileName = "r.jpg"

// GreenPicFileName is the green pic file name.
var GreenPicFileName = "g.jpg"

// BluePicFileName is the blue pic file name.
var BluePicFileName = "b.jpg"

// Enable Webcam Save Img if value is "on".
var enableWebcamSaveImg = testing.RegisterVarString(
	"utils.enableWebcamSaveImg",
	"off",
	"WWCB enable webcam save image",
)

// InitWebcam initializes Webcam.
func InitWebcam(ctx context.Context, s *testing.State) error {
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

// VerifyVideo is Verify by detect three color (R->G->B) video.
// Take frame from webcam every second.
// Verify is ok will return true, else return false
func VerifyVideo(ctx context.Context, s *testing.State, uid string, duration int) error {
	devPort, isHave := screenToCamera[uid]

	if !isHave {
		return errors.New("the uid " + uid + " not found")
	}

	cam, err := webcam.Open(devPort)
	if err != nil {
		return errors.New("webcam with '" + uid + "' open failed")
	}
	defer cam.Close()

	formatDesc := cam.GetSupportedFormats()
	for f := range formatDesc {
		if formatDesc[f] == "Motion-JPEG" {
			format := f
			_, _, _, err := cam.SetImageFormat(format, uint32(600), uint32(600))

			if err != nil {
				return errors.New("Set image format failed")
			}

			break
		}
	}

	err = cam.StartStreaming()
	if err != nil {
		return errors.New("webcam with '" + uid + "' streaming failed")
	}

	// 5 seconds time out.
	timeout := uint32(5)
	detectColorCount := 0
	startTime := time.Now().Unix()
	takeFrameTime := startTime

	for {
		err = cam.WaitForFrame(timeout)

		switch err.(type) {
		case nil:
		case *webcam.Timeout:
			return errors.New(err.Error())
		default:
			return errors.New("webcam with '" + uid + "' webcam take frame time out failed")
		}

		imgFileName := path.Join(s.OutDir(), "tmp.jpeg")
		if enableWebcamSaveImg.Value() == "on" {
			currentTime := time.Now()
			imgFileName = path.Join(s.OutDir(), devPort[5:]+"_verify_"+currentTime.Format("15:04:05")+".jpeg")
		}

		frame, err := cam.ReadFrame()
		if len(frame) != 0 {
			timeNow := time.Now().Unix()

			if int(timeNow-startTime) >= duration {
				return errors.New("verify fail")
			}

			distTime := timeNow - takeFrameTime

			if distTime >= 1 {
				err := os.WriteFile(imgFileName, frame, 0644)

				if err != nil {
					return errors.New("webcam with '" + uid + "' webcam write file error")
				}

				frameColor, err := GetColor(imgFileName)

				// some jpeg file from webcam need to add Dht
				if err != nil {
					frame = addMotionDht(frame)
					os.Remove(imgFileName)
					err := os.WriteFile(imgFileName, frame, 0644)
					if err != nil {
						return errors.New("webcam with '" + uid + "' webcam write file error")
					}

					frameColor, err = GetColor(imgFileName)

					if err != nil {
						testing.ContextLog(ctx, "jpeg file from webcam had decode issue")
						continue
					}
				}

				testing.ContextLog(ctx, "detect color: "+frameColor)
				if frameColor == detectVideoColor[detectColorCount] {
					detectColorCount++

					if detectColorCount > 2 {
						break
					}
				} else {
					if detectColorCount > 0 {
						if frameColor != detectVideoColor[detectColorCount-1] {
							detectColorCount = 0
						}
					}
				}

				takeFrameTime = timeNow
			}

		} else if err != nil {
			panic(err.Error())
		}
	}

	if detectColorCount <= 2 {
		return errors.New("verify fail")
	}

	return nil
}

// GetGamLightingValue is for detect lighting value by webcam.
// Value: 0(dark) ~ 255 (light).
func GetGamLightingValue(ctx context.Context, s *testing.State, uid string) (int, error) {
	devPort, isHave := screenToCamera[uid]

	if !isHave {
		return -1, errors.New("the webcam uid " + uid + " not found")
	}

	pixel, err := getAvgPixelFromWebcam(ctx, s, devPort, "LightingValue")

	if err != nil {
		return -1, err
	}

	return ((pixel.R + pixel.G + pixel.B) / 3), nil
}

// GetGamHotColdValue is for detect hot & cold value by webcam.
// Value: -255(cold) ~ 255 (hot).
func GetGamHotColdValue(ctx context.Context, s *testing.State, uid string) (int, error) {
	devPort, isHave := screenToCamera[uid]

	if !isHave {
		return -1, errors.New("the webcam uid " + uid + " not found")
	}

	pixel, err := getAvgPixelFromWebcam(ctx, s, devPort, "HotColdValue")

	if err != nil {
		return -1, err
	}

	return (pixel.R - pixel.B), nil
}

// addMotionDht is for add header to JPEG file.
func addMotionDht(frame []byte) []byte {
	jpegParts := bytes.Split(frame, sosMarker)
	return append(jpegParts[0], append(dhtMarker, append(dht, append(sosMarker, jpegParts[1]...)...)...)...)
}

// filterColorPixelValue is for get max pixel value by color.
func filterColorPixelValue(p Pixel) Pixel {
	colorStr := detectColor(p)

	if colorStr == "red" {
		return Pixel{p.R, 0, 0, 255}
	}

	if colorStr == "green" {
		return Pixel{0, p.G, 0, 255}
	}

	return Pixel{0, 0, p.B, 255}
}

// getAvgPixelFromWebcam is for get avg pixel from webcam.
func getAvgPixelFromWebcam(ctx context.Context, s *testing.State, devPort string, logStr string) (Pixel, error) {
	var p Pixel
	cam, err := webcam.Open(devPort)

	if err != nil {
		return Pixel{}, errors.New(devPort + " not found")
	}
	defer cam.Close()

	formatDesc := cam.GetSupportedFormats()
	for f := range formatDesc {
		if formatDesc[f] == "Motion-JPEG" {
			format := f
			_, _, _, err := cam.SetImageFormat(format, uint32(600), uint32(600))

			if err != nil {
				return Pixel{}, errors.New("Set image format failed")
			}

			break
		}
	}

	err = cam.StartStreaming()
	if err != nil {
		return Pixel{}, errors.New(devPort + " streaming failed")
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
			return Pixel{}, errors.New(devPort + " webcam take frame time out failed")
		}

		imgFileName := path.Join(s.OutDir(), "tmp.jpeg")
		if enableWebcamSaveImg.Value() == "on" {
			currentTime := time.Now()
			imgFileName = path.Join(s.OutDir(), devPort[5:]+"_"+logStr+"_"+currentTime.Format("15:04:05")+".jpeg")
		}

		frame, err := cam.ReadFrame()
		frameCount++

		if err != nil {
			return Pixel{}, err
		} else if frameCount > 10 && len(frame) != 0 {
			frame = addMotionDht(frame)
			err := os.WriteFile(imgFileName, frame, 0644)

			if err != nil {
				return Pixel{}, errors.New("get Pixel From Webcam write file error: " + devPort)
			}

			image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig)
			file, err := os.Open(imgFileName)

			if err != nil {
				return Pixel{}, errors.New("get Pixel From Webcam file open error: " + devPort)
			}

			defer file.Close()

			p, err = getAvgPixelColor(file)

			if err != nil {
				return Pixel{}, errors.New("get Pixel From Webcam error: " + devPort)
			}

			break
		}
	}

	return p, nil
}

// GetColor is for get color from JPEG file.
func GetColor(fileName string) (string, error) {
	image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig)
	file, err := os.Open(fileName)

	if err != nil {
		return "", errors.New("Error: File could not be opened")
	}

	defer file.Close()

	p, err := getAvgPixelColor(file)

	if err != nil {
		return "", errors.New("Error: Image could not be decoded:" + err.Error())
	}

	return detectColor(p), nil
}

// detectColor is for detect color from pixel.
func detectColor(p Pixel) string {
	redScore := int(scalarScore(p, redColor))
	greenScore := int(scalarScore(p, greenColor))
	blueScore := int(scalarScore(p, blueColor))

	if blueScore > redScore && blueScore > greenScore {
		return "blue"
	} else if greenScore > redScore {
		return "green"
	}

	return "red"
}

// distScore is for get the score from two Pixels.
// score more high means two pixels more difference.
func distScore(s1, s2 Pixel) float64 {
	score := (math.Abs(float64(s1.R-s2.R)) + math.Abs(float64(s1.G-s2.G)) + math.Abs(float64(s1.B-s2.B)))
	return score
}

// scalarScore is for get the score from two Pixels.
// score more high means two pixels more similar.
func scalarScore(s1, s2 Pixel) float64 {
	score := (math.Abs(float64(s1.R-s2.R))+math.Abs(float64(s1.G-s2.G))+math.Abs(float64(s1.B-s2.B)))/3*-1 + 255
	return score
}

// getAvgPixelColor is for get the bi-dimensional pixel array.
func getAvgPixelColor(file io.Reader) (Pixel, error) {
	img, _, err := image.Decode(file)

	if err != nil {
		fmt.Println(err.Error())
		return Pixel{}, err
	}

	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	var pixelsCount = 0
	var redSum float64
	var greenSum float64
	var blueSum float64
	for y := 0; y < height; y++ {
		var row []Pixel
		for x := 0; x < width; x++ {
			pixelXY := rgbaToPixel(img.At(x, y).RGBA())
			row = append(row, pixelXY)
			redSum += float64(pixelXY.R)
			greenSum += float64(pixelXY.G)
			blueSum += float64(pixelXY.B)
			pixelsCount++
		}
	}

	p := Pixel{int(redSum / float64(pixelsCount)), int(greenSum / float64(pixelsCount)), int(blueSum / float64(pixelsCount)), 255}

	return p, nil
}

// rgbaToPixel is for translate from rgb value to Pixel format.
func rgbaToPixel(r, g, b, a uint32) Pixel {
	return Pixel{int(r / 257), int(g / 257), int(b / 257), int(a / 257)}
}
