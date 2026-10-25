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
// throughput. The whole input is hashed: sampling a subset of a large document
// made the key blind to the unsampled bytes and served one document's cached
// result for another (see the call site in generateCacheKey).
//
// SECURITY: The random per-process seed makes key collisions infeasible to
// target, and hashing every byte ensures any modification — anywhere in the
// document — changes the key.
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
	if p.config.ExtractMetadata {
		flags |= 1 << 5
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

	// Hash the entire input. An earlier version sampled five windows (about
	// 4KB) of documents larger than 64KB, which made the key blind to every
	// byte outside those windows: two same-length documents differing only in
	// the unsampled middle produced the same key, so the second extraction was
	// served the first document's cached Result. maphash runs on AES-NI at
	// roughly 11-13 GB/s (measured), so even a 10MB document costs under a
	// millisecond — small against the parse and tree traversal it gates, and
	// correctness wins over the saved scan.
	_, _ = h.Write(data)

	// Produce a 128-bit key from the 64-bit maphash output. The second half
	// is derived via a splitmix64-style finalizer for additional diffusion.
	// The key is 128 bits wide but derived from a single 64-bit hash, so its
	// effective collision resistance is ~64 bits: the finalizer separates the
	// output space but does not double the collision entropy.
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
