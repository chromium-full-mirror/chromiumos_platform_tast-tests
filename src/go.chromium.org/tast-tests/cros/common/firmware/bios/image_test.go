// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bios

import (
	"testing"
)

func TestReadSectionData(t *testing.T) {
	s := map[ImageSection]SectionInfo{GBBImageSection: {1, 16}}
	i := Image{[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}, s}
	var flag uint32
	err := i.ReadSectionData(GBBImageSection, 12, 4, &flag)
	if err != nil {
		t.Fatal(err)
	}
	if flag != 0x04030201 {
		t.Fatalf("unexpected flags read %x from image %v", flag, i)
	}
}

func TestWriteSectionData(t *testing.T) {
	s := map[ImageSection]SectionInfo{GBBImageSection: {1, 16}}
	i := Image{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18}, s}

	var flag uint32
	flag = 0x04030201
	if err := i.WriteSectionData(GBBImageSection, 12, flag); err != nil {
		t.Fatal(err)
	}

	var got [18]byte
	copy(got[:], i.Data[:])

	want := [18]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 1, 2, 3, 4, 18}

	if got != want {
		t.Fatalf("image data incorrect, got: %v, want: %v", got, want)
	}
}

func TestShortGBBSection(t *testing.T) {
	s := map[ImageSection]SectionInfo{GBBImageSection: {0, 15}}
	i := Image{[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}, s}
	var flag uint32
	err := i.ReadSectionData(GBBImageSection, 12, 4, &flag)
	if err == nil {
		t.Fatal("Short section not detected: ", err)
	}
}

func TestGetLayout(t *testing.T) {
	s := map[ImageSection]SectionInfo{GBBImageSection: {1, 16}}
	i := Image{[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}, s}
	expectedLayout := "0x00000001:0x00000010 FV_GBB\n"
	layout := string(i.GetLayout())
	if layout != expectedLayout {
		t.Fatalf("unexpected layout, want %s, got %s", expectedLayout, layout)
	}
}
