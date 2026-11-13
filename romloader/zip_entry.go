// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package romloader

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sort"
)

const (
	// zipCheckpointSpacing is the least output between two checkpoints of a
	// deflated entry. A checkpoint is taken at the end of the decoder run
	// that reaches it, so a read decodes at most zipChunkMax to reach its
	// data.
	zipCheckpointSpacing = 1 << 20

	// zipChunkMax is the most output between two checkpoints. The run
	// that reaches a checkpoint has a whole window before it, because
	// zipCheckpointSpacing is larger than inflateBufSize, so it is at most
	// inflateRunMax.
	zipChunkMax = zipCheckpointSpacing + inflateRunMax

	// zipCacheSlots is the number of decoded regions kept per zip.
	zipCacheSlots = 16
)

// errZipChanged is returned by a read that finds an entry's data no longer
// lines up with its index.
var errZipChanged = errors.New("zip: data changed since it was indexed")

// zipCheckpoint is a position in a deflated entry that decoding can start
// from, with the output a later match may still refer to.
type zipCheckpoint struct {
	out    int64        // uncompressed offset
	at     inflatePoint // decoder position in the compressed data
	window []byte       // output preceding out, up to inflateWindow bytes
}

// zipChunk is the decoded data between one checkpoint of an entry and the
// next.
type zipChunk struct {
	entry *zipEntry
	index int
	data  []byte
	age   uint64
}

// zipDecoder is the decoding state shared by every deflated entry of one
// zip: the decoder and the cache of decoded regions. It is not safe for
// concurrent use, matching how a disc is read.
type zipDecoder struct {
	inf   inflater
	cache [zipCacheSlots]zipChunk
	age   uint64
}

// zipEntry gives random access to one deflated zip entry. A deflate stream
// can only be decoded forward, so the entry is indexed: decoded once, with a
// checkpoint recorded about every zipCheckpointSpacing bytes. A read decodes
// from the nearest checkpoint before it, and the decoded region is cached.
//
// The index is built a region at a time, in order, and is usable at every
// stage. ckpts always starts with the checkpoint at offset 0. Until indexed
// is set, the last checkpoint is where the index stops: buildIndex extends
// it to the end at once, and a read past it extends it as far as the read.
type zipEntry struct {
	name    string // entry name in the zip, for error messages
	r       io.ReaderAt
	off     int64  // offset of the compressed data in r
	csize   int64  // compressed size
	size    int64  // uncompressed size
	crc     uint32 // CRC-32 of the uncompressed data
	dec     *zipDecoder
	ckpts   []zipCheckpoint
	indexed bool   // ckpts covers the whole entry
	sum     uint32 // CRC-32 of the data before the last checkpoint
}

// extend decodes the region after the last checkpoint with inf, appending
// its data to data when keep is set. The region ends the entry, which sets
// indexed once the data is checked against the size and CRC-32 in the zip
// directory, or it ends at a new checkpoint. Its errors do not name the
// entry, which is left to the caller.
func (e *zipEntry) extend(inf *inflater, data []byte, keep bool) ([]byte, error) {
	last := e.ckpts[len(e.ckpts)-1]
	if err := inf.reset(e.r, e.off, e.csize, last.at, last.window); err != nil {
		return nil, err
	}
	total := last.out
	crc := e.sum
	for {
		out, err := inf.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		total += int64(len(out))
		if total > e.size {
			return nil, fmt.Errorf("data is longer than the %d bytes the zip directory gives", e.size)
		}
		crc = crc32.Update(crc, crc32.IEEETable, out)
		if keep {
			data = append(data, out...)
		}
		// A stream that ended in this run has no position left to resume
		// from. The next pass reports the end and checks the size.
		if total < e.size && !inf.done && total-last.out >= zipCheckpointSpacing {
			e.sum = crc
			e.ckpts = append(e.ckpts, zipCheckpoint{
				out:    total,
				at:     inf.point(),
				window: bytes.Clone(inf.window()),
			})
			return data, nil
		}
	}
	if total != e.size {
		return nil, fmt.Errorf("data is %d bytes, the zip directory gives %d", total, e.size)
	}
	if crc != e.crc {
		return nil, fmt.Errorf("CRC-32 %08x does not match %08x", crc, e.crc)
	}
	e.indexed = true
	return data, nil
}

// buildIndex indexes the whole entry from the start with inf. It touches
// nothing shared with other entries. Its errors do not name the entry, which
// is left to the caller.
func (e *zipEntry) buildIndex(inf *inflater) error {
	e.ckpts = []zipCheckpoint{{}}
	e.indexed = false
	e.sum = 0
	for !e.indexed {
		if _, err := e.extend(inf, nil, false); err != nil {
			return err
		}
	}
	return nil
}

// ReadAt reads the uncompressed data at off.
func (e *zipEntry) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("zip: negative read offset %d", off)
	}
	n := 0
	for n < len(p) {
		if off >= e.size {
			return n, io.EOF
		}
		// The last checkpoint at or before off.
		i := sort.Search(len(e.ckpts), func(k int) bool { return e.ckpts[k].out > off }) - 1
		data, err := e.chunk(i)
		if err != nil {
			return n, err
		}
		// When the index stopped at checkpoint i, its region was just
		// added and may end before off. The next pass finds the
		// checkpoint that follows it.
		rel := off - e.ckpts[i].out
		if rel >= int64(len(data)) {
			continue
		}
		c := copy(p[n:], data[rel:])
		n += c
		off += int64(c)
	}
	return n, nil
}

// chunk returns the decoded data from checkpoint i to the next checkpoint,
// or to the end of the entry for the last one. When the index stops at
// checkpoint i, decoding its region extends the index.
func (e *zipEntry) chunk(i int) ([]byte, error) {
	d := e.dec
	d.age++
	oldest := 0
	for s := range d.cache {
		c := &d.cache[s]
		if c.entry == e && c.index == i {
			c.age = d.age
			return c.data, nil
		}
		if c.age < d.cache[oldest].age {
			oldest = s
		}
	}

	// Reuse the evicted slot's buffer. A new one fits the largest region.
	c := &d.cache[oldest]
	c.entry = nil
	data := c.data[:0]
	if cap(data) < zipChunkMax {
		data = make([]byte, 0, zipChunkMax)
	}

	var err error
	if i == len(e.ckpts)-1 && !e.indexed {
		data, err = e.extend(&d.inf, data, true)
		if err != nil {
			return nil, fmt.Errorf("zip: track file %q: %w", e.name, err)
		}
	} else {
		data, err = e.decode(i, data)
		if err != nil {
			return nil, err
		}
	}

	*c = zipChunk{entry: e, index: i, data: data, age: d.age}
	return data, nil
}

// decode appends to data the region from checkpoint i, which the index
// already covers, to the next checkpoint or the end of the entry.
func (e *zipEntry) decode(i int, data []byte) ([]byte, error) {
	ck := &e.ckpts[i]
	end := e.size
	if i+1 < len(e.ckpts) {
		end = e.ckpts[i+1].out
	}
	want := int(end - ck.out)

	inf := &e.dec.inf
	if err := inf.reset(e.r, e.off, e.csize, ck.at, ck.window); err != nil {
		return nil, err
	}
	for len(data) < want {
		out, err := inf.next()
		// The index was built from this stream, so it ending before the
		// region is full means the data is not what was indexed.
		if err == io.EOF {
			return nil, errZipChanged
		}
		if err != nil {
			return nil, err
		}
		// A run may pass the end of the region.
		data = append(data, out[:min(len(out), want-len(data))]...)
	}
	return data, nil
}
