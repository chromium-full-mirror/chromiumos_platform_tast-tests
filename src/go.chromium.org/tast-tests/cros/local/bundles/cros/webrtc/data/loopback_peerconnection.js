// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// TestVisible holds JS variables to be queried by the tast test code.
class TestVisible {
  constructor() {
    this.localPeerConnections = [];
    this.remotePeerConnections = [];
  }
  setPeerConnections(localPCs, remotePCs) {
    this.localPeerConnections = localPCs;
    this.remotePeerConnections = remotePCs;
  }
}

let testVisible = new TestVisible;

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
  const isSModeEnc = svcScalabilityMode.startsWith('S');
  let localPC = new RTCPeerConnection({ encodedInsertableStreams: isSModeEnc });
  let remotePC = new RTCPeerConnection({
    encodedInsertableStreams: isSModeEnc,
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

  testVisible.setPeerConnections([localPC], [remotePC]);
}

async function startSMode(profile, width, height, svcScalabilityMode) {
  let { stream, localPC, remotePC } = await createStreamAndPCs(
    width,
    height,
    svcScalabilityMode,
    ''
  );
  let localPCTransceiver = localPC.addTransceiver(stream.getVideoTracks()[0], {
    // Prefer resolution even at the cost of visual quality to avoid falling
    // down to SW video encoding, see b/181320567 or crbug.com/1179020.
    degradationPreference: 'maintain-resolution',
    streams: [stream],
    sendEncodings: [{ scalabilityMode: svcScalabilityMode }],
  });
  // TODO(crbug.com/1513866): Remove this header extension setting.
  setUpHeaderExtension(localPCTransceiver);
  let localPCStream = localPCTransceiver.sender.createEncodedStreams();

  const numStreams = parseInt(svcScalabilityMode[1]);
  let localPCs = new Array(numStreams);
  let remotePCs = new Array(numStreams);
  let remoteVideoIds = new Array(numStreams);
  let clonedLocalPCWriters = new Array(numStreams - 1);

  for (let i = 0; i < numStreams; i++) {
    remoteVideoIds[i] = 'remoteVideo' + i;
    if (i > 0) {
      let videoElement = document.createElement('video');
      videoElement.id = remoteVideoIds[i];
      videoElement.autoplay = true;
      videoElement.muted = true;
      document.getElementById('container').append(videoElement);
    }
  }

  for (let i = 0; i < numStreams - 1; i++) {
    const clonedLocalPC = new RTCPeerConnection({
      encodedInsertableStreams: true,
    });
    let clonedLocalPCTransceiver = clonedLocalPC.addTransceiver('video');
    // TODO(crbug.com/1513866): Remove this header extension setting.
    setUpHeaderExtension(clonedLocalPCTransceiver);
    let clonedLocalPCWriter = clonedLocalPCTransceiver.sender
      .createEncodedStreams()
      .writable.getWriter();
    localPCs[i] = clonedLocalPC;
    clonedLocalPCWriters[i] = clonedLocalPCWriter;
    remotePCs[i] = new RTCPeerConnection({ encodedInsertableStreams: true });
  }
  localPCs[numStreams - 1] = localPC;
  remotePCs[numStreams - 1] = remotePC;

  const topSpatialLayerIndex = numStreams - 1;
  let onTracks = new Array(numStreams);
  for (let i = 0; i < numStreams; i++) {
    const onTrack = new Promise((resolve, reject) => {
      remotePCs[i].ontrack = (e) => {
        let remoteVideo = document.getElementById('remoteVideo' + i);
        remoteVideo.srcObject = new MediaStream([e.track]);
        let receiver = e.receiver;
        let receiverStream = receiver.createEncodedStreams();
        receiverStream.readable
          .pipeThrough(
            new TransformStream({
              transform(frame, controller) {
                const metadata = frame.getMetadata();
                // TODO(bugs.webrtc.org/15795): Set the marker bit of RtcPacket
                // and RemotePC[i] decodes frames whose spatial indices are i.
                if (metadata.spatialIndex == topSpatialLayerIndex) {
                  controller.enqueue(frame);
                }
              },
            })
          )
          .pipeTo(receiverStream.writable);
        resolve();
      };
    });
    onTracks[i] = onTrack;
  }

  let ssrcs = new Array(numStreams - 1);
  for (let i = 0; i < numStreams; i++) {
    const ssrc = await connect(localPCs[i], remotePCs[i], profile, 0, []);
    if (i < ssrcs.length) {
      ssrcs[i] = ssrc;
    }
  }

  localPCStream.readable
    .pipeThrough(
      new TransformStream({
        transform(frame, controller) {
          const metadata = frame.getMetadata();
          for (let i = 0; i < ssrcs.length; i++) {
            const clonedFrame = structuredClone(frame);
            const modifiedMetadata = structuredClone(metadata);
            modifiedMetadata.synchronizationSource = ssrcs[i];
            clonedFrame.setMetadata(modifiedMetadata);
            clonedLocalPCWriters[i].write(clonedFrame);
          }
          controller.enqueue(frame);
        },
      })
    )
    .pipeTo(localPCStream.writable);

  for (let i = 0; i < onTracks.length; i++) {
    await onTracks[i];
  }

  testVisible.setPeerConnections(localPCs, remotePCs);
}
