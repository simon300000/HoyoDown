package main

import "github.com/klauspost/compress/zstd"

// zstdDecompress is deliberately pure Go so the same source builds on
// Windows, Linux and macOS without a C compiler or a system libzstd.
func zstdDecompress(src []byte, expectedSize int64) ([]byte, error) {
	dst := make([]byte, 0)
	if expectedSize > 0 {
		dst = make([]byte, 0, expectedSize)
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	return decoder.DecodeAll(src, dst)
}
