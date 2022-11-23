# Pepper plugin for testing video decoding

This directory contains the data files used by the Tast test for exercising
video decoding in Pepper.

## Plugin preparation

The plugin used by this test is compiled from the `video_decode.cc` example in
the NaCl SDK. It can be found in the Chromium repository and the link below
points to the exact version used as a start point:

[`native_client_sdk/src/examples/api/video_decode/video_decode.cc`](https://chromium.googlesource.com/chromium/src/+/07edf8593383fe7963d4aa10e1fa4a0e2ac6dfcb/native_client_sdk/src/examples/api/video_decode/video_decode.cc)

This file was modified in order to make the plugin send a message to
the Tast test when the video finishes. Without this change the
test will never know when the video finished playing. These changes are
available in the `video_decode.patch` file that can be found in this folder.

The `video_decode.cc` example is precompiled and injected into the test via data
files, since compiling it requires setting up the NaCl toolchain (which would
be cumbersome and very time-consuming to perform during the test).

In accordance with the Tast guidelines, only small textual files are stored in
this repository, meanwhile the actual `video_decode` plugin binary
(`video_decode.pexe`) is stored as an external data file. If a new version of it
needs to be generated, the following command could be used (which assumes that
you have the Chromium checkout and the NaCl toolchain set up):

```
${NACL_SDK_ROOT}/toolchain/linux_pnacl/bin/pnacl-clang++ \
  -I${NACL_SDK_ROOT}/include \
  -L${NACL_SDK_ROOT}/lib/pnacl/Release \
  ${CHROME_SRC_ROOT}/native_client_sdk/src/examples/api/video_decode/video_decode.cc \
  -O2 -lppapi -lppapi_cpp \
  -o video_decode.non_finalized.pexe \
  && \
  ${NACL_SDK_ROOT}/toolchain/linux_pnacl/bin/pnacl-finalize \
  video_decode.non_finalized.pexe \
  -o video_decode.pexe
```

Remember to apply the changes in `video_decode.patch` mentioned above, otherwise the
test will not work properly.
