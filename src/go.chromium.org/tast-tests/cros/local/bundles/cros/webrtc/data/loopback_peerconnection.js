// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

let remotePeerConnection;
let localPeerConnection;

async function getStream(width, height, displayMediaType) {
  let constraints = {
    audio: false,
    video: {
      width: width,
      height: height,
      framerate: 30,
    },
  };
  if (displayMediaType !== '') {
    constraints.video.displaySurface = displayMediaType;
    constraints.selfBrowserSurface = 'include';
  }

  let stream;
  if (constraints.video.displaySurface) {
    stream = await navigator.mediaDevices.getDisplayMedia(constraints);
    constraints.framerate = { min: 30, max: 30 };
    stream.getVideoTracks()[0].applyConstraints(constraints);
  } else {
    stream = await navigator.mediaDevices.getUserMedia(constraints);
  }
  return stream;
}

async function createStreamAndPCs(
  width,
  height,
  svcScalabilityMode,
  displayMediaType
) {
  const isSmodeEnc = svcScalabilityMode.startsWith('S');
  let localPC = new RTCPeerConnection({ encodedInsertableStreams: isSmodeEnc });
  let remotePC = new RTCPeerConnection({
    encodedInsertableStreams: isSmodeEnc,
  });

  return {
    stream: await getStream(width, height, displayMediaType),
    localPC: localPC,
    remotePC: remotePC,
  };
}

async function start(
  profile,
  width,
  height,
  simulcasts,
  svcScalabilityMode,
  displayMediaType
) {
  let { stream, localPC, remotePC } = await createStreamAndPCs(
    width,
    height,
    svcScalabilityMode,
    displayMediaType
  );

  let rids = [];
  let init = {
    // Prefer resolution even at the cost of visual quality to avoid falling
    // down to SW video encoding, see b/181320567 or crbug.com/1179020.
    degradationPreference: 'maintain-resolution',
    streams: [stream],
  };
  if (simulcasts > 1) {
    for (let i = 0; i < simulcasts; i++) {
      rids.push(i);
    }
    init.sendEncodings = rids.map((i) => {
      return { rid: i, scaleResolutionDownBy: 2 ** (rids.length - (i + 1)) };
    });
  } else if (svcScalabilityMode !== '') {
    // TODO: Decode the top spatial layer only in LxTx_KEY.
    init.sendEncodings = [{ scalabilityMode: svcScalabilityMode }];
  }
  localPC.addTransceiver(stream.getVideoTracks()[0], init);
  remotePC.addTransceiver('video');

  const onTrack = new Promise((resolve, reject) => {
    remotePC.ontrack = (e) => {
      const remoteVideo = document.getElementById('remoteVideo0');
      remoteVideo.srcObject = e.streams[0];
      resolve();
    };
  });

  // |targetBitrate| uses a conservative 0.05 bits per pixel (bpp) estimate.
  const targetBitrate = width * height * 30 /*fps*/ * 0.05;
  await connect(localPC, remotePC, profile, targetBitrate, rids);
  await onTrack;

  // Set to global peer connection variables so that the golang test code is
  // able to query it.
  localPeerConnection = localPC;
  remotePeerConnection = remotePC;
}
