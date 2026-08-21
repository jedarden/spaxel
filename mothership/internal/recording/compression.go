// Package recording provides compression utilities for CSI frame chunks.
package recording

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/klauspost/compress/zstd"
)

const (
	// CompressionVersion1 is the version marker for compressed chunks.
	CompressionVersion1 byte = 1

	// DefaultChunkSize is the default target size for compressed chunks.
	// Chunks are compressed when they reach this size or older than the timeout.
	DefaultChunkSize = 64 * 1024 // 64 KB

	// CompressionLevel is the zstd compression level (3 for fast decompression).
	// Level 3 provides good compression ratio with very fast decode speed,
	// which is important for the replay use case (read-heavy).
	CompressionLevel = 3
)

// ChunkHeader is the header for a compressed chunk.
//
// Layout (16 bytes):
//   magic[4]        "CSCZ" (Compressed Spaxel Chunk)
//   version[1]       1 (for compressed chunks)
//   frameCount[2]    uint16 LE - number of frames in this chunk
//   uncompressedLen[4] uint32 LE - total size of uncompressed data
//   reserved[5]      zero padding
// Followed by zstd-compressed frame data.
type ChunkHeader struct {
	Magic           [4]byte
	Version         byte
	FrameCount      uint16
	UncompressedLen uint32
}

const chunkMagic = "CSCZ"

// Compressor batches frames and compresses them using zstd.
// Frames are added with AddFrame() and compressed when the chunk reaches
// the target size or when Flush() is called.
type Compressor struct {
	zenc            *zstd.Encoder
	buf             *bytes.Buffer
	targetChunkSize int
	frameCount      int
}

// NewCompressor creates a new Compressor with the given target chunk size.
// Pass 0 for targetChunkSize to use DefaultChunkSize.
func NewCompressor(targetChunkSize int) (*Compressor, error) {
	if targetChunkSize <= 0 {
		targetChunkSize = DefaultChunkSize
	}

	zenc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(CompressionLevel)))
	if err != nil {
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}

	return &Compressor{
		zenc:            zenc,
		buf:             &bytes.Buffer{},
		targetChunkSize: targetChunkSize,
	}, nil
}

// AddFrame adds a frame to the current chunk. Returns the compressed chunk
// bytes (including header) if the chunk reached the target size, nil otherwise.
func (c *Compressor) AddFrame(recvTimeNS int64, frame []byte) ([]byte, error) {
	// Write timestamp (8 bytes), frame length (2 bytes), and frame data
	binary.Write(c.buf, binary.LittleEndian, uint64(recvTimeNS))
	binary.Write(c.buf, binary.LittleEndian, uint16(len(frame)))
	c.buf.Write(frame)
	c.frameCount++

	// Check if we should compress this chunk
	if c.buf.Len() >= c.targetChunkSize {
		return c.Flush()
	}
	return nil, nil
}

// Flush compresses the current chunk and returns the compressed bytes.
// Returns nil if the chunk is empty.
func (c *Compressor) Flush() ([]byte, error) {
	if c.frameCount == 0 {
		return nil, nil
	}

	uncompressed := c.buf.Bytes()
	uncompressedLen := len(uncompressed)

	// Compress - EncodeAll compresses and returns the encoded data
	encoded := c.zenc.EncodeAll(uncompressed, nil)

	// Build header
	header := ChunkHeader{
		Magic:           [4]byte{'C', 'S', 'C', 'Z'},
		Version:         CompressionVersion1,
		FrameCount:      uint16(c.frameCount),
		UncompressedLen: uint32(uncompressedLen),
	}

	// Build result: chunk length prefix + header + compressed data
	dataLen := 16 + len(encoded) // header + compressed data
	chunkLen := 2 + dataLen      // total including prefix
	result := make([]byte, chunkLen)
	binary.LittleEndian.PutUint16(result[0:2], uint16(dataLen)) // Store length of header+data (not including prefix)
	copy(result[2:6], header.Magic[:])
	result[6] = header.Version
	binary.LittleEndian.PutUint16(result[7:9], header.FrameCount)
	binary.LittleEndian.PutUint32(result[9:13], header.UncompressedLen)
	// bytes 13-17 are reserved padding (already zeroed)
	copy(result[18:], encoded)

	// Reset for next chunk
	c.buf.Reset()
	c.frameCount = 0

	return result, nil
}

// Decompressor decompresses chunks created by Compressor.
type Decompressor struct {
	zdec *zstd.Decoder
	buf  *bytes.Buffer
}

// NewDecompressor creates a new Decompressor.
func NewDecompressor() (*Decompressor, error) {
	zdec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("create zstd decoder: %w", err)
	}

	return &Decompressor{
		zdec: zdec,
		buf:  &bytes.Buffer{},
	}, nil
}

// DecompressChunk decompresses a chunk and calls frameFn for each frame.
// The chunk data should include the 16-byte header.
// frameFn receives (recvTimeNS, frame) for each frame.
func (d *Decompressor) DecompressChunk(chunk []byte, frameFn func(recvTimeNS int64, frame []byte) error) error {
	if len(chunk) < 16 {
		return fmt.Errorf("chunk too short: %d bytes", len(chunk))
	}

	// Parse header
	var header ChunkHeader
	copy(header.Magic[:], chunk[0:4])
	header.Version = chunk[4]
	header.FrameCount = binary.LittleEndian.Uint16(chunk[5:7])
	header.UncompressedLen = binary.LittleEndian.Uint32(chunk[7:11])
	// bytes 11-15 are reserved padding

	if string(header.Magic[:]) != chunkMagic {
		return fmt.Errorf("invalid chunk magic: %s", string(header.Magic[:]))
	}
	if header.Version != CompressionVersion1 {
		return fmt.Errorf("unsupported chunk version: %d", header.Version)
	}

	compressed := chunk[16:]
	if len(compressed) == 0 {
		return fmt.Errorf("no compressed data in chunk")
	}

	// Decompress
	decompressed, err := d.zdec.DecodeAll(compressed, nil)
	if err != nil {
		return fmt.Errorf("zstd decompress: %w", err)
	}

	if len(decompressed) != int(header.UncompressedLen) {
		return fmt.Errorf("decompressed size mismatch: got %d, want %d",
			len(decompressed), header.UncompressedLen)
	}

	// Iterate frames
	offset := 0
	for i := 0; i < int(header.FrameCount); i++ {
		// Read timestamp (8 bytes)
		if offset+8 > len(decompressed) {
			return fmt.Errorf("truncated timestamp at offset %d", offset)
		}
		recvTimeNS := int64(binary.LittleEndian.Uint64(decompressed[offset : offset+8]))
		offset += 8

		// Read frame length (2 bytes)
		if offset+2 > len(decompressed) {
			return fmt.Errorf("truncated frame length at offset %d", offset)
		}
		frameLen := int(binary.LittleEndian.Uint16(decompressed[offset : offset+2]))
		offset += 2

		if offset+frameLen > len(decompressed) {
			return fmt.Errorf("truncated frame data at offset %d (need %d, have %d)",
				offset, frameLen, len(decompressed)-offset)
		}

		frame := decompressed[offset : offset+frameLen]
		if err := frameFn(recvTimeNS, frame); err != nil {
			return err
		}

		offset += frameLen
	}

	return nil
}

// Close releases resources.
func (d *Decompressor) Close() error {
	d.zdec.Close()
	return nil
}

// Close releases resources.
func (c *Compressor) Close() error {
	c.zenc.Close()
	return nil
}
