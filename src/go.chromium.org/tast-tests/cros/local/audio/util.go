// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package audio interacts with audio operation.
package audio

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type audioFileFormat int

const (
	raw audioFileFormat = iota
	wav
)

// TestRawData is used to specify parameters of the audio test data, which should be raw, signed, and little-endian.
type TestRawData struct {
	// Path specifies the file path of audio data.
	Path string
	// BitsPerSample specifies bits per data sample.
	BitsPerSample int
	// Channels specifies the channel count of audio data.
	Channels int
	// Rate specifies the sampling rate.
	Rate int
	// Frequencies specifies the frequency of each channel, whose length should be equal to Channels.
	// This is only used in the sine tone generation of sox.
	Frequencies []int
	// Volume specifies the volume scale of sox, e.g. 0.5 to scale volume by half. -1.0 to invert.
	// This is only used in the sine tone generation of sox.
	Volume float32
	// Duration specifies the duration of audio data in seconds.
	Duration int
}

// ConvertRawToWav converts the audio raw file to wav file.
func ConvertRawToWav(ctx context.Context, rawData TestRawData, wavFileName string) error {
	err := testexec.CommandContext(
		ctx, "sox",
		"-b", strconv.Itoa(rawData.BitsPerSample),
		"-r", strconv.Itoa(rawData.Rate),
		"-c", strconv.Itoa(rawData.Channels),
		"-e", "signed",
		"-t", "raw",
		rawData.Path, wavFileName).Run(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "sox failed")
	}
	return nil
}

// TrimFileFrom removes all samples before startTime from the file.
func TrimFileFrom(ctx context.Context, oldFileName, newFileName string, startTime time.Duration) error {
	err := testexec.CommandContext(
		ctx, "sox", oldFileName, newFileName, "trim",
		strconv.FormatFloat(startTime.Seconds(), 'f', -1, 64)).Run(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "sox failed")
	}
	return nil
}

// TrimFileFromAndTo removes all samples before startTime and after startTIme + length from the file.
func TrimFileFromAndTo(ctx context.Context, oldFileName, newFileName string, startTime, length time.Duration) error {
	err := testexec.CommandContext(
		ctx, "sox", oldFileName, newFileName, "trim",
		strconv.FormatFloat(startTime.Seconds(), 'f', -1, 64),
		strconv.FormatFloat(length.Seconds(), 'f', -1, 64)).Run(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "sox failed")
	}
	return nil
}

// CheckRecordingNotZero checks the recording file to see whether internal mic works normally.
// The recorded samples can not be all zeros. It is impossible for a normal internal mic.
//
//	TODO (b/250472324): Re-enable the checking for internal static noise after we found a solution (either audiobox, or through filtering)
func CheckRecordingNotZero(ctx context.Context, fileName string) error {
	out, err := testexec.CommandContext(ctx, "sox", fileName, "-n", "stats").CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "sox failed")
	}

	re := regexp.MustCompile("RMS Pk dB +\\S+ +(\\S+) +(\\S+)")
	rms := re.FindStringSubmatch(string(out))
	if rms == nil {
		testing.ContextLog(ctx, "sox stats: ", string(out))
		return errors.New("could not find RMS info from the sox result")
	}

	rmsLeft, err := strconv.ParseFloat(rms[1], 32)
	if err != nil {
		return errors.Wrap(err, "atof failed")
	}
	rmsRight, err := strconv.ParseFloat(rms[2], 32)
	if err != nil {
		return errors.Wrap(err, "atof failed")
	}

	testing.ContextLogf(ctx, "Left channel RMS: %f dB", rmsLeft)
	testing.ContextLogf(ctx, "Right channel RMS: %f dB", rmsRight)

	// If all samples are zeros, the rms is -inf.
	if math.IsInf(rmsLeft, -1) || math.IsInf(rmsRight, -1) {
		return errors.New("the samples are all zeros")
	}

	return nil
}

func getRmsAmplitude(ctx context.Context, inputArgs []string) (float64, error) {
	args := append(inputArgs, "-n", "stat")
	cmd := testexec.CommandContext(ctx, "sox", args...)
	_, bstderr, err := cmd.SeparatedOutput()
	if err != nil {
		return 0.0, errors.Wrap(err, "sox failed")
	}
	stderr := string(bstderr)

	re := regexp.MustCompile("RMS\\s+amplitude:\\s+(\\S+)")
	match := re.FindStringSubmatch(stderr)
	if match == nil {
		testing.ContextLog(ctx, "sox stat: ", stderr)
		return 0.0, errors.New("could not find RMS info from the sox result")
	}

	rms, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0.0, errors.Wrap(err, "atof failed")
	}

	return rms, nil
}

// GetRmsAmplitude gets signal RMS of testData by sox.
func GetRmsAmplitude(ctx context.Context, testData TestRawData) (float64, error) {
	return getRmsAmplitude(
		ctx,
		[]string{
			"-b", strconv.Itoa(testData.BitsPerSample),
			"-c", strconv.Itoa(testData.Channels),
			"-r", strconv.Itoa(testData.Rate),
			"-e", "signed",
			"-t", "raw",
			testData.Path,
		},
	)
}

// GetRmsAmplitudeFromWav gets signal RMS of a wav file by sox.
func GetRmsAmplitudeFromWav(ctx context.Context, filepath string) (float64, error) {
	return getRmsAmplitude(
		ctx,
		[]string{
			filepath,
		},
	)
}

func generateSineData(ctx context.Context, testData TestRawData, fileFormat audioFileFormat) error {
	if len(testData.Frequencies) != testData.Channels {
		return errors.Errorf("unexpected length of frequencies: got %d; want %d", len(testData.Frequencies), testData.Channels)
	}

	args := []string{
		"-n",
		"-b", strconv.Itoa(testData.BitsPerSample),
		"-c", strconv.Itoa(testData.Channels),
		"-r", strconv.Itoa(testData.Rate),
		"-e", "signed",
	}
	if fileFormat == raw {
		args = append(args, "-t", "raw")
	}
	args = append(args,
		testData.Path,
		"synth", strconv.Itoa(testData.Duration),
	)

	for _, f := range testData.Frequencies {
		args = append(args, "sine", strconv.Itoa(f))
	}
	args = append(args, "vol", fmt.Sprintf("%f", testData.Volume))

	if err := testexec.CommandContext(ctx, "sox", args...).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "sox failed")
	}
	return nil
}

// GenerateTestRawData generates sine raw data by sox with specified parameters in testData, and stores in testData.Path.
func GenerateTestRawData(ctx context.Context, testData TestRawData) error {
	return generateSineData(ctx, testData, raw)
}

// GenerateTestWavData generates sine data by sox with specified parameters in wavData, and stores in wavData.Path with .wav file format
func GenerateTestWavData(ctx context.Context, wavData TestRawData) error {
	return generateSineData(ctx, wavData, wav)
}

// ReadS16LEPCM reads S16LE PCM data from file, and returns the data in arr[channel][x].
func ReadS16LEPCM(path string, channels int) ([][]int16, error) {
	// Read file
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open file")
	}
	defer f.Close()

	// Each sample is 16-bit signed integer.
	// Data order: <ch 1 sample 1> <ch 2 sample 1> ... <ch 1 sample 2> ...
	arr := make([][]int16, channels)
	channel := 0
	var samp int16
	for {
		if err := binary.Read(f, binary.LittleEndian, &samp); err != nil {
			if err == io.EOF {
				break
			}
			return nil, errors.Wrap(err, "error while reading file")
		}
		arr[channel] = append(arr[channel], samp)
		channel = (channel + 1) % channels
	}
	return arr, nil
}

// CalculateFrequency calculates the frequency of a recorded raw audio.
// The method used here is the same as `sox stat` rough frequency.
func CalculateFrequency(data []float64, sampleRate float64) float64 {
	var dsum2, sum2, last float64
	for i := range data {
		samp := data[i] / math.MaxInt32 // Downsampling to 32-bit.
		if i == 0 {
			last = samp
		}
		sum2 += samp * samp
		delta := samp - last
		dsum2 += delta * delta
		last = samp
	}
	if sum2 == 0 {
		// Return -1 if all data are zeros, because we could not calculate frequency.
		return -1
	}
	return math.Sqrt(dsum2/sum2) * sampleRate / (math.Pi * 2)
}

// CountFrequencyErrors count the number of unexpected audio's frequency.
// It slices the input and calculates the frequency for each slices.
// If the number of slices with unexpected frequency exceeds incorrectLimit, returns error.
// It ignores slices in the beginning that contain only zeros and the first slice with non-zero data.
// The number of all-zero slices must be less than `startingSlicesLimit`, or it returns error.
// The difference from CheckFrequency is this function returns the number of incorrect slices, instead of returning error
func CountFrequencyErrors(ctx context.Context, data []int16, sampleRate, expectedFreq, freqTolerance float64) (int, error) {
	const (
		samplesPerSlice     = 1000 // Number of samples per slice. 1000 on 48kHz = 21ms
		startingSlicesLimit = 24   // Max starting slices allowed. 24 on 48kHz = 500ms
	)

	hasNonZeroData := func(data []int16) bool {
		for _, d := range data {
			if d != 0 {
				return true
			}
		}
		return false
	}

	isStarting := true
	startingSlices := 0
	incorrectSlices := 0

	// Loop through each slice of data. Ignore the last slice if there is not enough data.
	for i := 0; i+samplesPerSlice <= len(data); i += samplesPerSlice {
		dataSlice := data[i : i+samplesPerSlice]
		if isStarting {
			if hasNonZeroData(dataSlice) {
				// First slice with non-zero data, still ignore this slice and start checking at the next slice.
				isStarting = false
			} else {
				startingSlices++
				if startingSlices > startingSlicesLimit {
					return 0, errors.New("reached starting slices limit")
				}
			}
		} else {
			dataFloat := make([]float64, samplesPerSlice)
			for j := range dataSlice {
				dataFloat[j] = float64(dataSlice[j])
			}

			freq := CalculateFrequency(dataFloat, sampleRate)
			if math.Abs(freq-expectedFreq) > freqTolerance {
				testing.ContextLogf(ctx, "Slice %d frequency incorrect. expect:%.2f got:%.2f", i/samplesPerSlice, expectedFreq, freq)
				incorrectSlices++
			}
		}
	}

	if isStarting {
		return 0, errors.New("not enough data to get out of starting phase")
	}

	return incorrectSlices, nil
}

// CheckFrequency checks if the audio's frequency is expected.
// It slices the input and calculates the frequency for each slices.
// If the number of slices with unexpected frequency exceeds incorrectLimit, returns error.
// It ignores slices in the beginning that contain only zeros and the first slice with non-zero data.
// The number of all-zero slices must be less than `startingSlicesLimit`, or it returns error.
func CheckFrequency(ctx context.Context, data []int16, sampleRate, expectedFreq, freqTolerance float64, incorrectLimit int) error {
	if incorrectSlices, err := CountFrequencyErrors(ctx, data, sampleRate, expectedFreq, freqTolerance); err != nil {
		return err
	} else if incorrectSlices > incorrectLimit {
		return errors.Errorf("incorrect slices count over limit, incorrect slices: %v, limit: %v", incorrectSlices, incorrectLimit)
	}
	return nil
}

// CheckRecordedFrequency similar to CheckFrequency but uses DUT's check_recorded_frequency.py script
// TODO(b/309390542): Don't use this function unless migrating from autotest.
// This script has functional partiy with autotest method of checking frequency.
// The audioFormat.frequencies contains the frequency to check. for example [440, 440] means
// 440 frequency in first channel and 440 frequency in second channel.
func CheckRecordedFrequency(
	ctx context.Context,
	recordingFile *os.File,
	audioFormat *TestRawData,
) error {

	if audioFormat.BitsPerSample != 16 && audioFormat.BitsPerSample != 32 {
		return errors.Errorf("CheckRecordedFrequency only supports bits per sample of 16 or 32 got %d", audioFormat.BitsPerSample)
	}

	if audioFormat.Channels == 0 {
		return errors.Errorf("Missing argument channels %d", audioFormat.Channels)
	}

	if audioFormat.Rate == 0 {
		return errors.Errorf("Missing argument rate %d", audioFormat.Rate)
	}

	if audioFormat.Frequencies == nil {
		return errors.New("Missing argument frequencies")
	}

	args := []string{
		"-t", recordingFile.Name(),
		// format 16 -> S16_LE and 32 -> S32_LE
		"-f", "S" + strconv.Itoa(audioFormat.BitsPerSample) + "_LE",
		"-c", strconv.Itoa(audioFormat.Channels),
		"-r", strconv.Itoa(audioFormat.Rate),
	}

	// golden file frequencies
	args = append(args, "-g")
	for _, f := range audioFormat.Frequencies {
		args = append(args, strconv.Itoa(f))
	}

	// frequency map usually directly ie. playing channel i -> recording channel i
	args = append(args, "-m")
	for i := range audioFormat.Frequencies {
		args = append(args, strconv.Itoa(i))
	}

	cmd := testexec.CommandContext(ctx, "check_recorded_frequency.py", args...)

	cmdOutputBytes, err := cmd.CombinedOutput(testexec.DumpLogOnError)
	cmdOutput := string(cmdOutputBytes)
	testing.ContextLogf(ctx, "Output for check_recorded_frequency.py: %s", cmdOutput)

	if err != nil {
		if cmdOutput != "" {
			if strings.Contains(cmdOutput, "is away from golden") {
				errorMsg := cmdOutput[strings.LastIndex(cmdOutput, ":")+1:]
				return errors.Wrapf(err, "Frequency does not match: %s", errorMsg)
			}
			return errors.Wrap(err, "Command check_recorded_frequency.py returns error")
		}
		return errors.Wrap(err, "Unable to run command check_recorded_frequency.py")
	}
	testing.ContextLogf(ctx, "Obtained frequencies: %s", cmdOutput)

	return nil
}
