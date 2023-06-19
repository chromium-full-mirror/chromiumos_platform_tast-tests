SELECT name, ts, dur, args.key, args.value_type, args.int_value, args.string_value
FROM slices
LEFT JOIN args ON slices.arg_set_id == args.arg_set_id
WHERE name in (
      'RTCVideoEncoder::Encode',
      'MojoVideoEncodeAccelerator::Encode',
      'MojoVideoEncodeAcceleratorService::Encode',
      'V4L2VEA::EncodeTask',
      'VAVEA::EncodeTask',

      'V4L2VEA::BitstreamBufferReady',
      'VAVEA::BitstreamBufferReady',
      'MojoVideoEncodeAcceleratorService::BitstreamBufferReady',
      'RTCVideoEncoder::Impl::BitstreamBufferReady',

      'RTCVideoEncoder::Impl::EncodeOneFrame', -- Duration
      'RTCVideoEncoder::Impl::EncodeOneFrameWithNativeInput', -- Duration
      'V4L2VEA::ImageProcessor::Process', -- Duration
      'VAVEA::ImageProcessor::BlitSurface', -- Duration
      'MojoVEAService::EncodingFrameDuration', -- Duration
      'PlatformEncoding.Encode', -- Duration

      'MojoVideoDecoder::Decode',
      'VideoDecoderPipeline::Decode',
      'VideoDecoderPipeline::DecodeTask',
      'VideoDecoderPipeline::OnFrameDecoded',
      'VideoDecoderPipeline::OnFrameProcessed',
      'VideoDecoderPipeline::OnFrameConverted',
      'MojoVideoDecoder::OnVideoFrameDecoded',

      'MojoDecoderBufferWriter::Write', -- Duration
      'MojoDecoderBufferReader::Read'  -- Duration
)
ORDER BY ts
