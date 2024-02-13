// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cliprectperf provides constants and structs for clip rect perf tests.
package cliprectperf

import (
	"strings"
	"text/template"

	"go.chromium.org/tast/core/errors"
)

// ClipRectParam contains information needed to run ClipRect perf test.
type ClipRectParam struct {
	Variant string
}

var videoParams = map[string]interface{}{
	// Name of the video to play.
	"vidName": "buck1080p60_h264.mp4",
	// Lower the native video dimension since 1920x1080 is too big.
	// 1024x576 is a commonly used 16:9 resolution.
	"vidWidth":  1024,
	"vidHeight": 576,
	// Margin size between videos.
	"margin": 25,
}

// GetScript returns the script for the perf test variant.
func GetScript(variant string) (string, error) {
	var tmpl string
	if variant == "video" {
		tmpl = singleVideoTmpl
	} else if variant == "multi_video" {
		tmpl = fiveVideosTmpl
	} else {
		return "", errors.Errorf("unsupported test variant: %s", variant)
	}
	t := template.Must(template.New("script").Parse(tmpl))
	builder := &strings.Builder{}
	if err := t.Execute(builder, videoParams); err != nil {
		return "", errors.Wrap(err, "failed to parse script")
	}
	return builder.String(), nil
}

// singleVideoTmpl sets up the page with a singular video with half
// clipped outside of the window.
const singleVideoTmpl = `
	toggleVideo("{{.vidName}}");
	const vids = document.querySelectorAll("[id='{{.vidName}}']");
	let v = vids[0];
	v.style = "width: {{.vidWidth}}px; {{.vidHeight}}: 576px; clear:both;";
	v.muted = true;
	let videoBound = v.getBoundingClientRect();
	let pxToHalf = videoBound.top + (videoBound.height / 2);
	document.body.style.marginTop = String(-pxToHalf) + "px";
`

// fiveVideosTmpl sets up the page with 5 videos where each video is
// behind a rectengular mask with the border clipped.
const fiveVideosTmpl = `
	for (let i = 0; i < 5; i++){
		addVideo("{{.vidName}}");
	}
	const vids = document.querySelectorAll("[id='{{.vidName}}']");
	let divTop = document.createElement("div");
	let divBot = document.createElement("div");
	divTop.className = "row";
	divBot.className = "row";

	let ratio = screen.height / screen.width;
	let divWidth = (screen.width - ({{.margin}} * 4)) / 3;
	let divHeight = divWidth * ratio;
	let vidLeft = "-" + String(({{.vidWidth}} - divWidth) / 2) + "px;";
	let vidTop = "-" + String(({{.vidHeight}} - divHeight) / 2) + "px;";
	divWidth = String(divWidth) + "px;";
	divHeight = String(divHeight) + "px;";


	for (let i = 0; i < 5; i++) {
		d = document.createElement("div");
		d.className = "outer";
		d.style = "width: " + divWidth + "height: " + divHeight;
		d.appendChild(vids[i]);
		vids[i].muted = true;
		vids[i].style = "left: " + vidLeft + "top: " + vidTop;
		if (i < 3) {
			divTop.appendChild(d);
		} else {
			divBot.appendChild(d);
		}
	}

	let styleSheet = document.createElement("style");
	styleSheet.innerText =` + "`" + `
	.outer {
		overflow: hidden;
		position: relative;
		margin-left: {{.margin}}px;
		margin-bottom: {{.margin}}px;
	}
	.row {
		display: flex;
		justify-content: center;
	}
	video {
		position: relative;
		width: {{.vidWidth}}px;
		height: {{.vidHeight}}px;
	}` + "`;" + `
	document.head.appendChild(styleSheet);
	videoCont = document.getElementById("video-container");
	videoCont.appendChild(divTop);
	videoCont.appendChild(divBot);
`
