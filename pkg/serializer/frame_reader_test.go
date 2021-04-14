package serializer

import (
	"io"
	"io/ioutil"
	"reflect"
	"strings"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

const (
	fooYAML = `kind: Foo
apiVersion: bar/v1
a: b1234567890
c: d1234567890
e: f1234567890
hello: true`

	barYAML = `kind: Bar
apiVersion: foo/v1
a: b1234567890
c: d1234567890
e: f1234567890
hello: false`

	bazYAML = `baz: true`

	testYAML = "\n---\n" + fooYAML + "\n---\n" + barYAML + "\n---\n" + bazYAML
)

func Test_FrameReader_ReadFrame(t *testing.T) {
	testYAMLReadCloser := json.YAMLFramer.NewFrameReader(ioutil.NopCloser(strings.NewReader(testYAML)))

	type fields struct {
		rc           io.ReadCloser
		bufSize      int
		maxFrameSize int
	}
	type result struct {
		wantB   []byte
		wantErr bool
	}
	tests := []struct {
		name   string
		fields fields
		wants  []result
	}{
		{
			name: "three-document YAML case",
			fields: fields{
				rc:           testYAMLReadCloser,
				bufSize:      16,
				maxFrameSize: 1024,
			},
			wants: []result{
				{
					wantB:   []byte(fooYAML),
					wantErr: false,
				},
				{
					wantB:   []byte(barYAML),
					wantErr: false,
				},
				{
					wantB:   []byte(bazYAML),
					wantErr: false,
				},
				{
					wantB:   nil,
					wantErr: true,
				},
			},
		},
		{
			name: "maximum size reached",
			fields: fields{
				rc:           testYAMLReadCloser,
				bufSize:      16,
				maxFrameSize: 32,
			},
			wants: []result{
				{
					wantB:   nil,
					wantErr: true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rf := &frameReader{
				rc:           tt.fields.rc,
				rcMu:         &sync.Mutex{},
				bufSize:      tt.fields.bufSize,
				maxFrameSize: tt.fields.maxFrameSize,
			}
			for _, expected := range tt.wants {
				gotB, err := rf.ReadFrame()
				if (err != nil) != expected.wantErr {
					t.Errorf("frameReader.ReadFrame() error = %v, wantErr %v", err, expected.wantErr)
					return
				}
				if len(gotB) < len(expected.wantB) {
					t.Errorf("frameReader.ReadFrame(): got smaller slice %v than expected %v", gotB, expected.wantB)
					return
				}
				if !reflect.DeepEqual(gotB[:len(expected.wantB)], expected.wantB) {
					t.Errorf("frameReader.ReadFrame() = %v, want %v", gotB, expected.wantB)
				}
			}
		})
	}
}
