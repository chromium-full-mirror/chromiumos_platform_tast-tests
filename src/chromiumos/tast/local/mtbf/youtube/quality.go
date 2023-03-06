// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package youtube

// Quality is the option in youtube "Settings / Quality" menu
// for changing video playback quality.
type Quality string

// Definitions of the quality options in the youtube "Settings / Quality" menu.
const (
	Quality4320P   Quality = "4320p"
	Quality4320P60 Quality = "4320p60"
	Quality2160P   Quality = "2160p"
	Quality2160P60 Quality = "2160p60"
	Quality1440P   Quality = "1440p"
	Quality1440P60 Quality = "1440p60"
	Quality1080P   Quality = "1080p"
	Quality1080P60 Quality = "1080p60"
	Quality720P    Quality = "720p"
	Quality720P60  Quality = "720p60"
	Quality480P    Quality = "480p"
	Quality360P    Quality = "360p"
	Quality240P    Quality = "240p"
	Quality144P    Quality = "144p"
)

// Definitions of the quality options position.
const (
	QualityOption1 = iota + 1
	QualityOption2
	QualityOption3
	QualityOption4
	QualityOption5
	QualityOption6
	QualityOption7
	QualityOption8
	QualityOption9
)

// qualityOptionMap defines the quality options position from bottom to top.
var qualityOptionMap = map[Quality]int{
	Quality144P:    QualityOption1,
	Quality240P:    QualityOption2,
	Quality360P:    QualityOption3,
	Quality480P:    QualityOption4,
	Quality720P:    QualityOption5,
	Quality720P60:  QualityOption5,
	Quality1440P:   QualityOption6,
	Quality1440P60: QualityOption6,
	Quality1080P:   QualityOption7,
	Quality1080P60: QualityOption7,
	Quality2160P:   QualityOption8,
	Quality2160P60: QualityOption8,
	Quality4320P:   QualityOption9,
	Quality4320P60: QualityOption9,
}
