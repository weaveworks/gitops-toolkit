/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file provides a means to read one whole frame from an io.ReadCloser
// returned by a k8s.io/apimachinery/pkg/runtime.Framer.NewFrameReader()
//
// This code is (temporarily) forked and derived from
// https://github.com/kubernetes/apimachinery/blob/v0.21.2/pkg/runtime/serializer/streaming/streaming.go
// and will be upstreamed if maintainers allow. The reason for forking this
// small piece of code is two-fold: a) This functionality is bundled within
// a runtime.Decoder, not provided as "just" some type of Reader, b) The
// upstream doesn't allow to configure the maximum frame size.

package frame

import (
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
)

// k8sStreamingReader is an interface used to access the reading frames from the underlying
// io.ReadCloser. This interface should eventually make its way into Kubernetes.
type k8sStreamingReader interface {
	Read() ([]byte, error)
	io.Closer
}

// Ref: https://github.com/kubernetes/apimachinery/blob/v0.21.2/pkg/runtime/serializer/streaming/streaming.go#L63-L67
func newK8sStreamingReader(rc io.ReadCloser, maxFrameSize int64) k8sStreamingReader {
	if maxFrameSize == 0 {
		maxFrameSize = DefaultMaxFrameSize
	}

	return &k8sStreamingReaderImpl{
		reader: rc,
		buf:    make([]byte, 1024),
		// CHANGE: maxBytes is configurable
		maxBytes: maxFrameSize,
	}
}

// Ref: https://github.com/kubernetes/apimachinery/blob/v0.21.2/pkg/runtime/serializer/streaming/streaming.go#L51-L57
type k8sStreamingReaderImpl struct {
	reader io.ReadCloser
	buf    []byte
	// CHANGE: In the original code, maxBytes was an int. int64 is more specific and flexible, however.
	maxBytes  int64
	resetRead bool
}

// Ref: https://github.com/kubernetes/apimachinery/blob/v0.21.2/pkg/runtime/serializer/streaming/streaming.go#L75-L106
func (d *k8sStreamingReaderImpl) Read() ([]byte, error) {
	base := 0
	for {
		n, err := d.reader.Read(d.buf[base:])
		if err == io.ErrShortBuffer {
			if n == 0 {
				return nil, fmt.Errorf("got short buffer with n=0, base=%d, cap=%d", base, cap(d.buf))
			}
			if d.resetRead {
				continue
			}
			// double the buffer size up to maxBytes
			// NOTE: This might need changing upstream eventually, it only works when
			// d.maxBytes/len(d.buf) is a multiple of 2
			// CHANGE: In the original code no cast from int -> int64 was needed
			bufLen := int64(len(d.buf))
			if bufLen < d.maxBytes {
				base += n
				// CHANGE: Instead of unconditionally doubling the buffer, double the buffer
				// length only to the extent it fits within d.maxBytes. Previously, it was a
				// requirement that d.maxBytes was a multiple of 1024 for this logic to work.
				newBytes := len(d.buf)
				if d.maxBytes < 2*bufLen {
					newBytes = int(d.maxBytes - bufLen)
				}
				d.buf = append(d.buf, make([]byte, newBytes)...)
				continue
			}
			// must read the rest of the frame (until we stop getting ErrShortBuffer)
			d.resetRead = true
			// base = 0 // CHANGE: Not needed (as pointed out by golangci-lint:ineffassign)
			return nil, streaming.ErrObjectTooLarge
		}
		if err != nil {
			return nil, err
		}
		if d.resetRead {
			// now that we have drained the large read, continue
			d.resetRead = false
			continue
		}
		base += n
		break
	}
	return d.buf[:base], nil
}

func (d *k8sStreamingReaderImpl) Close() error { return d.reader.Close() }
