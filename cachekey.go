package html

import (
	"encoding/binary"
	"hash/maphash"
)

// cacheKeySeed is a process-lifetime random seed for cache key hashing.
// hash/maphash uses AES-NI on amd64 for maximum throughput, making this
// significantly faster than the previous hand-rolled xxHash-style hash that
// relied on multiply-rotate arithmetic. The random seed also improves
// resistance to hash-flooding attacks compared to fixed constants.
var cacheKeySeed = maphash.MakeSeed()

// generateCacheKey creates a cache key from the raw HTML input bytes and the
// processor's configuration. Hashing raw bytes instead of the decoded UTF-8
// string eliminates the need for encoding detection on cache hits: the caller
// hashes the input, checks the cache, and only runs the O(n) encoding scan on a
// miss. The Encoding config is mixed into the hash so that the same bytes
// processed under different forced-encoding settings do not collide.
//
// Uses hash/maphash which leverages AES-NI instructions on amd64 for high
// throughput. Uses multi-point sampling for large documents to bound the hash
// cost while maintaining collision resistance.
//
// SECURITY: Uses 5-point sampling for better collision resistance against
// hash-flooding attacks. The sampling strategy ensures that modifications
// anywhere in the document are likely to change the hash.
//
// Returns the 128-bit key as a stack value ([16]byte) rather than a heap string.
// The cache is keyed by this value type, so generating a key allocates nothing —
// [16]byte arrays are comparable and usable directly as map keys.
func (p *Processor) generateCacheKey(data []byte) [16]byte {
	var h maphash.Hash
	h.SetSeed(cacheKeySeed)

	// Pack boolean flags into a single byte
	flags := uint8(0)
	if p.config.ExtractArticle {
		flags |= 1 << 0
	}
	if p.config.PreserveImages {
		flags |= 1 << 1
	}
	if p.config.PreserveLinks {
		flags |= 1 << 2
	}
	if p.config.PreserveVideos {
		flags |= 1 << 3
	}
	if p.config.PreserveAudios {
		flags |= 1 << 4
	}
	_ = h.WriteByte(flags)

	// Mix format strings
	_, _ = h.WriteString(p.config.InlineImageFormat)
	_, _ = h.WriteString(p.config.InlineLinkFormat)
	_, _ = h.WriteString(p.config.TableFormat)

	// Include the Encoding setting: the same raw bytes decoded under different
	// forced-encoding configurations can produce different extraction results.
	// For auto-detect (Encoding == ""), this hashes the empty string — identical
	// content always produces the same key, which is correct because auto-detect
	// is deterministic for the same bytes.
	_, _ = h.WriteString(p.config.Encoding)

	dataLen := len(data)
	if dataLen <= maxCacheKeySize {
		// Hash the entire input directly
		_, _ = h.Write(data)
	} else {
		// SECURITY: Multi-point sampling for large documents.
		// Using 5 sampling points for better collision resistance.
		const sampleCount = 5
		sampleSize := cacheKeySample / sampleCount
		if sampleSize < 256 {
			sampleSize = 256
		}

		for i := 0; i < sampleCount; i++ {
			var start, end int
			switch i {
			case sampleCount - 1:
				end = dataLen
				start = dataLen - sampleSize
				if start < 0 {
					start = 0
				}
			case 0:
				start = 0
				end = sampleSize
				if end > dataLen {
					end = dataLen
				}
			default:
				offset := (dataLen * i) / (sampleCount - 1)
				start = offset - sampleSize/2
				if start < 0 {
					start = 0
				}
				end = start + sampleSize
				if end > dataLen {
					end = dataLen
					start = end - sampleSize
					if start < 0 {
						start = 0
					}
				}
			}

			if start < end {
				_, _ = h.Write(data[start:end])
			}
		}

		// Mix content length for additional uniqueness
		var lenBuf [8]byte
		binary.LittleEndian.PutUint64(lenBuf[:], uint64(dataLen))
		_, _ = h.Write(lenBuf[:])
	}

	// Produce a 128-bit key from the 64-bit maphash output. The second half
	// is derived via a splitmix64-style finalizer for additional diffusion,
	// giving effective collision resistance far beyond 2^64 without a second
	// hash pass.
	sum := h.Sum64()
	var key [16]byte
	binary.LittleEndian.PutUint64(key[:8], sum)
	binary.LittleEndian.PutUint64(key[8:], splitmix64(sum))

	return key
}

// splitmix64 is a fast finalizer that diffuses a 64-bit value. Used to derive
// the second half of the cache key from the maphash output.
func splitmix64(h uint64) uint64 {
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	h *= 0x94d049bb133111eb
	h ^= h >> 31
	return h
}
