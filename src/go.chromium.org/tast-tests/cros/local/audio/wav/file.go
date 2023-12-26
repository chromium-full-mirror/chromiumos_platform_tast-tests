// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wav

import (
	"context"
	"encoding/binary"
	"io"
	"os"

	"go.chromium.org/tast/core/errors"
)

const (
	// headerSize is the size of the header of a WAV file.
	headerSize = 44

	// wavFormatPCM is the indicator of a PCM WAV file in the format field.
	wavFormatPCM = 1
)

// Header is a struct that represents the header of a WAV file.
type Header struct {
	Channels      uint16
	SampleRate    uint32
	BitsPerSample uint16
	format        uint16
	DataSize      uint32
	FrameCount    uint32
}

// File is a struct that represents a WAV file.
type File struct {
	Header Header
	Body   [][]int32
}

func getHeader(headerBytes []byte) (Header, error) {
	if string(headerBytes[:4]) != "RIFF" || string(headerBytes[8:12]) != "WAVE" {
		return Header{}, errors.New("File is not a valid wav file")
	}

	header := Header{}
	header.format = binary.LittleEndian.Uint16(headerBytes[20:22])
	header.Channels = binary.LittleEndian.Uint16(headerBytes[22:24])
	header.SampleRate = binary.LittleEndian.Uint32(headerBytes[24:28])
	header.BitsPerSample = binary.LittleEndian.Uint16(headerBytes[34:36])
	header.DataSize = binary.LittleEndian.Uint32(headerBytes[40:44])

	sampleSize := header.BitsPerSample / 8
	frameSize := sampleSize * header.Channels
	header.FrameCount = header.DataSize / uint32(frameSize)

	return header, nil
}

// ReadPCMFile reads a PCM wav file and returns a File struct.
func ReadPCMFile(ctx context.Context, filepath string) (File, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return File{}, err
	}
	defer file.Close()

	var headerBytes [headerSize]byte
	if _, err := io.ReadFull(file, headerBytes[:]); err != nil {
		return File{}, err
	}
	header, err := getHeader(headerBytes[:])
	if err != nil {
		return File{}, err
	}
	if header.format != wavFormatPCM {
		return File{}, errors.New("File is not a PCM wav file")
	}

	pcmData := make([][]int32, header.Channels)
	for i := range pcmData {
		pcmData[i] = make([]int32, header.FrameCount)
	}

	for frame := range pcmData[0] {
		for channel := range pcmData {
			switch header.BitsPerSample {
			case 16:
				var sample int16
				err := binary.Read(file, binary.LittleEndian, &sample)
				if err != nil {
					return File{}, err
				}
				pcmData[channel][frame] = int32(sample)
			case 32:
				var sample int32
				err := binary.Read(file, binary.LittleEndian, &sample)
				if err != nil {
					return File{}, err
				}
				pcmData[channel][frame] = int32(sample)
			default:
				return File{}, errors.Errorf("unsupported bits per sample: %d", header.BitsPerSample)
			}
		}
	}

	return File{Header: header, Body: pcmData}, nil
}
