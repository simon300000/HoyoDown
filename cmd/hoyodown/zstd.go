package main

import "github.com/klauspost/compress/zstd"

// zstdDecoder is reused across all decompression calls. DecodeAll is
// safe for concurrent use once the decoder has been warmed up.
var zstdDecoder, _ = zstd.NewReader(nil, zstd.WithDecoderConcurrency(0))

// zstdDecompress is deliberately pure Go so the same source builds on
// Windows, Linux and macOS without a C compiler or a system libzstd.
func zstdDecompress(src []byte, expectedSize int64) ([]byte, error) {
	dst := make([]byte, 0)
	if expectedSize > 0 {
		dst = make([]byte, 0, expectedSize)
	}
	return zstdDecoder.DecodeAll(src, dst)
}
