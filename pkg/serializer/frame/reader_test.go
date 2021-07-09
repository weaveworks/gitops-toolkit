package frame

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: Test the output traces more througoutly, when there is SpanProcessor that supports writing
// relevant data to a file, and do matching between spans.

// TODO: Make some 16M (or more) JSON/YAML files and show that these are readable (or not). That's not
// testing a case that already isn't tested by the unit tests below, but would be a good marker that
// it actually solves the right problem.

// TODO: Maybe add some race-condition tests? The centralized place mutexes are used are in
// highlevel{Reader,Writer}, so that'd be the place in that case.

type testcase struct {
	readOpts  []ReaderOption
	writeOpts []WriterOption

	name     string
	testdata []testdata
	// Reader.ReadFrame will be called len(readResults) times. If a err == nil return is expected, just put
	// nil in the error slice. Similarly for Writer.WriteFrame and writeResults.
	// Note that len(readResults) >= len(frames) and len(writeResults) >= len(frames) must hold.
	// By issuing more reads or writes than there are frames, one can check the error behavior
	readResults  []error
	writeResults []error
	// if closeWriterIdx or closeReaderIdx are non-nil, the Reader/Writer will be closed after the read at
	// that specified index. closeWriterErr and closeReaderErr can be used to check the error returned by
	// the close call.
	closeWriterIdx *int64
	closeWriterErr error
	//expectWriterClosed bool
	closeReaderIdx *int64
	closeReaderErr error
	//expectReaderCloser bool
}

type testdata struct {
	ct FramingType
	// frames contain the individual frames of rawData, which in turn is the content of the underlying
	// source/stream. if len(writeResults) == 0, there will be no checking that writing all frames
	// in order will produce the correct rawData. if len(readResults) == 0, there will be no checking
	// that reading rawData will produce the frames string
	rawData string
	frames  []string
}

const (
	yamlSep       = "---\n"
	noNewlineYAML = `foobar: true`
	testYAML      = noNewlineYAML + "\n"
	testYAMLlen   = int64(len(testYAML))
	messyYAMLP1   = `
---

---
` + noNewlineYAML + `
`
	messyYAMLP2 = `

---
---
` + noNewlineYAML + `
---`
	messyYAML = messyYAMLP1 + messyYAMLP2

	testJSON    = `{"foo": true}`
	testJSONlen = int64(len(testJSON))
	testJSON2   = `{"bar": "hello"}`
	messyJSONP1 = `

` + testJSON + `
`
	messyJSONP2 = `

` + testJSON + `
`
	messyJSON = messyJSONP1 + messyJSONP2

	otherCT       = FramingType("other")
	otherFrame    = "('other'; 9)\n('bar'; true)"
	otherFrameLen = int64(len(otherFrame))
)

func TestReader(t *testing.T) {
	// Some tests depend on this
	require.Equal(t, testYAMLlen, testJSONlen)
	NewFactoryTester(t, DefaultFactory{}).Test()
}

// TODO: Test that closing of Readers and Writers works

var defaultTestCases = []testcase{
	// Roundtrip cases
	{
		name: "simple roundtrip",
		testdata: []testdata{
			{ct: FramingTypeYAML, frames: []string{testYAML}, rawData: yamlSep + testYAML},
			{ct: FramingTypeJSON, frames: []string{testJSON}, rawData: testJSON},
		},
		writeResults: []error{nil, nil, nil, nil},
		readResults:  []error{nil, io.EOF, io.EOF, io.EOF},
	},

	{
		name: "two-frame roundtrip with closed writer",
		testdata: []testdata{
			{ct: FramingTypeYAML, frames: []string{testYAML, testYAML}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: FramingTypeJSON, frames: []string{testJSON, testJSON2}, rawData: testJSON + testJSON2},
		},
		writeResults: []error{nil, nil, nil, nil},
		readResults:  []error{nil, nil, io.EOF, io.EOF},
	},
	// YAML newline addition
	{
		name: "YAML Read: a newline will be added",
		testdata: []testdata{
			{ct: FramingTypeYAML, rawData: noNewlineYAML, frames: []string{testYAML}},
		},
		readResults: []error{nil, io.EOF},
	},
	{
		name: "YAML Write: a newline will be added",
		testdata: []testdata{
			{ct: FramingTypeYAML, frames: []string{noNewlineYAML}, rawData: yamlSep + testYAML},
		},
		writeResults: []error{nil},
	},
	// Empty frames
	{
		name: "Read: io.EOF when there are no non-empty frames",
		testdata: []testdata{
			{ct: FramingTypeYAML, rawData: "---"},
			{ct: FramingTypeYAML, rawData: "---\n"},
			{ct: FramingTypeJSON, rawData: ""},
			{ct: FramingTypeJSON, rawData: "    \n    "},
		},
		readResults: []error{io.EOF},
	},
	{
		name: "Write: Empty sanitized frames aren't written",
		testdata: []testdata{
			{ct: FramingTypeYAML, frames: []string{"---", "---\n", " \n--- \n---"}},
			{ct: FramingTypeJSON, frames: []string{"", "    \n    ", "  "}},
		},
		writeResults: []error{nil, nil, nil},
	},
	{
		name: "Write: can write empty frames forever without errors",
		testdata: []testdata{
			{ct: FramingTypeYAML, frames: []string{testYAML, testYAML}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: FramingTypeJSON, frames: []string{testJSON, testJSON2}, rawData: testJSON + testJSON2},
		},
		writeResults: []error{nil, nil, nil, nil, nil},
		readResults:  []error{nil, nil, io.EOF},
	},
	// Sanitation
	{
		name: "YAML Read: a leading \\n--- will be ignored",
		testdata: []testdata{
			{ct: FramingTypeYAML, rawData: "\n" + yamlSep + noNewlineYAML, frames: []string{testYAML}},
		},
		readResults: []error{nil, io.EOF},
	},
	{
		name: "YAML Read: a leading --- will be ignored",
		testdata: []testdata{
			{ct: FramingTypeYAML, rawData: yamlSep + noNewlineYAML, frames: []string{testYAML}},
		},
		readResults: []error{nil, io.EOF},
	},
	{
		name: "Read: sanitize messy content",
		testdata: []testdata{
			{ct: FramingTypeYAML, rawData: messyYAML, frames: []string{testYAML, testYAML}},
			{ct: FramingTypeJSON, rawData: messyJSON, frames: []string{testJSON, testJSON}},
		},
		readResults: []error{nil, nil, io.EOF},
	},
	{
		name: "Write: sanitize messy content",
		testdata: []testdata{
			{ct: FramingTypeYAML, frames: []string{messyYAMLP1, messyYAMLP2}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: FramingTypeJSON, frames: []string{messyJSONP1, messyJSONP2}, rawData: testJSON + testJSON},
		},
		writeResults: []error{nil, nil},
	},
	// MaxFrameSize
	{
		name: "Read: the frame size is exactly within bounds, also enforce counter reset",
		testdata: []testdata{
			{ct: FramingTypeYAML, rawData: testYAML + yamlSep + testYAML, frames: []string{testYAML, testYAML}},
			{ct: FramingTypeJSON, rawData: testJSON + testJSON, frames: []string{testJSON, testJSON}},
		},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrameSize: testYAMLlen}},
		readResults: []error{nil, nil, io.EOF},
	},
	{
		name: "Read: the frame is out of bounds, on the same line",
		testdata: []testdata{
			{ct: FramingTypeYAML, rawData: testYAML},
			{ct: FramingTypeJSON, rawData: testJSON},
		},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrameSize: testYAMLlen - 1}},
		readResults: []error{ErrFrameSizeOverflow},
	},
	{
		name: "YAML Read: the frame is out of bounds, but continues on the next line",
		testdata: []testdata{
			{ct: FramingTypeYAML, rawData: testYAML + testYAML},
		},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrameSize: testYAMLlen}},
		readResults: []error{ErrFrameSizeOverflow},
	},
	{
		name: "Read: first frame ok, then always frame overflow",
		testdata: []testdata{
			{ct: FramingTypeYAML, rawData: testYAML + yamlSep + testYAML + testYAML, frames: []string{testYAML}},
			{ct: FramingTypeJSON, rawData: testJSON + testJSON2, frames: []string{testJSON}},
		},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrameSize: testYAMLlen}},
		readResults: []error{nil, ErrFrameSizeOverflow, ErrFrameSizeOverflow, ErrFrameSizeOverflow},
	},
	{
		name: "Write: the second frame is too large, ignore that, but allow writing smaller frames later",
		testdata: []testdata{
			{ct: FramingTypeYAML, frames: []string{testYAML, testYAML + testYAML, testYAML}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: FramingTypeJSON, frames: []string{testJSON, testJSON2, testJSON}, rawData: testJSON + testJSON},
		},
		writeOpts:    []WriterOption{&ReaderWriterOptions{MaxFrameSize: testYAMLlen}},
		writeResults: []error{nil, ErrFrameSizeOverflow, nil},
	},
	{
		name: "first frame ok, then Read => EOF and Write => nil consistently",
		testdata: []testdata{
			{ct: FramingTypeYAML, frames: []string{testYAML}, rawData: yamlSep + testYAML},
			{ct: FramingTypeJSON, frames: []string{testJSON}, rawData: testJSON},
		},
		readResults:  []error{nil, io.EOF, io.EOF, io.EOF, io.EOF},
		writeResults: []error{nil, nil, nil, nil, nil},
	},
	// MaxFrames
	{
		name: "Write: Don't allow writing more than a maximum amount of frames",
		testdata: []testdata{
			{ct: FramingTypeYAML, frames: []string{testYAML, testYAML, testYAML}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: FramingTypeJSON, frames: []string{testJSON, testJSON, testJSON}, rawData: testJSON + testJSON},
		},
		writeResults: []error{nil, nil, ErrFrameCountOverflow, ErrFrameCountOverflow},
		writeOpts:    []WriterOption{&ReaderWriterOptions{MaxFrames: 2}},
	},
	{
		name: "Read: Don't allow reading more than a maximum amount of successful frames",
		testdata: []testdata{
			{ct: FramingTypeYAML,
				rawData: testYAML + yamlSep + testYAML + yamlSep + testYAML,
				frames:  []string{testYAML, testYAML}},
			{ct: FramingTypeJSON,
				rawData: testJSON + testJSON + testJSON,
				frames:  []string{testJSON, testJSON}},
		},
		readResults: []error{nil, nil, ErrFrameCountOverflow, ErrFrameCountOverflow},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrames: 2}},
	},
	{
		name: "Read: Don't allow reading more than a maximum amount of successful frames, and 10x in total",
		testdata: []testdata{
			{ct: FramingTypeYAML,
				rawData: strings.Repeat("\n"+yamlSep, 10) + testYAML},
		},
		readResults: []error{ErrFrameCountOverflow, ErrFrameCountOverflow},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrames: 1}},
	},
	{
		name: "Read: Allow reading up to the maximum amount of 10x the successful frames count",
		testdata: []testdata{
			{ct: FramingTypeYAML,
				rawData: strings.Repeat("\n"+yamlSep, 9) + testYAML + yamlSep + yamlSep, frames: []string{testYAML}},
		},
		readResults: []error{nil, ErrFrameCountOverflow, ErrFrameCountOverflow},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrames: 1}},
	},
	{
		name: "Read: Allow reading exactly that amount of successful frames, if then io.EOF",
		testdata: []testdata{
			{ct: FramingTypeYAML,
				rawData: testYAML + yamlSep + testYAML,
				frames:  []string{testYAML, testYAML}},
			{ct: FramingTypeJSON,
				rawData: testJSON + testJSON,
				frames:  []string{testJSON, testJSON}},
		},
		readResults: []error{nil, nil, io.EOF, io.EOF},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrames: 2}},
	},
	// Other Content Types
	{
		name: "Roundtrip: Allow reading other content types when MaxFrames == 1, check overflows too",
		testdata: []testdata{
			{ct: otherCT, rawData: otherFrame, frames: []string{otherFrame}},
		},
		writeResults: []error{nil, ErrFrameCountOverflow, ErrFrameCountOverflow, ErrFrameCountOverflow},
		readResults:  []error{nil, io.EOF, io.EOF, io.EOF},
		writeOpts:    []WriterOption{&ReaderWriterOptions{MaxFrames: 1}},
		readOpts:     []ReaderOption{&ReaderWriterOptions{MaxFrames: 1}},
	},
	{
		name: "Read: other content type frame size is exactly within bounds",
		testdata: []testdata{
			{ct: otherCT, rawData: otherFrame, frames: []string{otherFrame}},
		},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrameSize: otherFrameLen, MaxFrames: 1}},
		readResults: []error{nil, io.EOF},
	},
	{
		name: "Read: other content type frame size overflow",
		testdata: []testdata{
			{ct: otherCT, rawData: otherFrame},
		},
		readOpts:    []ReaderOption{&ReaderWriterOptions{MaxFrameSize: otherFrameLen - 1, MaxFrames: 1}},
		readResults: []error{ErrFrameSizeOverflow, io.EOF, io.EOF},
	},
	{
		name: "Write: other content type frame size overflow",
		testdata: []testdata{
			{ct: otherCT, frames: []string{otherFrame, otherFrame}},
		},
		writeOpts:    []WriterOption{&ReaderWriterOptions{MaxFrameSize: otherFrameLen - 1, MaxFrames: 1}},
		writeResults: []error{ErrFrameSizeOverflow, ErrFrameSizeOverflow, nil},
	},
}

func NewFactoryTester(t *testing.T, f Factory) *FactoryTester {
	return &FactoryTester{
		t:       t,
		factory: f,
		cases:   defaultTestCases,
	}
}

type FactoryTester struct {
	t       *testing.T
	factory Factory

	cases []testcase
}

func (h *FactoryTester) Test() {
	for _, c := range h.cases {
		h.t.Run(c.name, func(t *testing.T) {
			h.testRoundtripCase(t, &c)
		})
	}
}

func (h *FactoryTester) testRoundtripCase(t *testing.T, c *testcase) {
	// "Compile" the reader and writer options into structs for introspection
	wOpts := defaultWriterOpts().ApplyOptions(c.writeOpts)
	rOpts := defaultReaderOpts().ApplyOptions(c.readOpts)

	for i, data := range c.testdata {
		t.Run(fmt.Sprintf("%d %s", i, data.ct), func(t *testing.T) {
			h.testRoundtripCaseFramingType(t, c, &data, wOpts, rOpts)
		})
	}
}

func (h *FactoryTester) testRoundtripCaseFramingType(t *testing.T, c *testcase, d *testdata, wOpts *WriterOptions, rOpts *ReaderOptions) {
	var buf bytes.Buffer

	readCloseCounter := &recordingCloser{}
	writeCloseCounter := &recordingCloser{}
	w := h.factory.NewWriter(d.ct, ioWriteCloser{&buf, writeCloseCounter}, wOpts)
	assert.Equalf(t, w.FramingType(), d.ct, "Writer.FramingType")
	r := h.factory.NewReader(d.ct, ioReadCloser{&buf, readCloseCounter}, rOpts)
	assert.Equalf(t, r.FramingType(), d.ct, "Reader.FramingType")
	ctx := context.Background()

	// Write frames using the writer
	for i, expected := range c.writeResults {
		var frame []byte
		// Only write a frame using the writer if one is supplied
		if i < len(d.frames) {
			frame = []byte(d.frames[i])
		}

		// Write the frame using the writer and check the error
		got := w.WriteFrame(ctx, frame)
		assert.ErrorIsf(t, got, expected, "Writer.WriteFrame err %d", i)

		// If we should close the writer here, do it and check the expected error
		if c.closeWriterIdx != nil && *c.closeWriterIdx == int64(i) {
			assert.ErrorIsf(t, w.Close(ctx), c.closeWriterErr, "Writer.Close err %d", i)
		}
	}

	assert.Equalf(t, 0, writeCloseCounter.count, "Writer should not be closed")

	// Check that the written output was as expected, if writing is enabled
	if len(c.writeResults) != 0 {
		assert.Equalf(t, d.rawData, buf.String(), "Writer Output")
	} else {
		// If writing was not tested, make sure the buffer contains the raw data for reading
		buf = *bytes.NewBufferString(d.rawData)
	}

	// Read frames using the reader
	for i, expected := range c.readResults {
		// Check the expected error
		frame, err := r.ReadFrame(ctx)
		assert.ErrorIsf(t, err, expected, "Reader.ReadFrame err %d", i)
		// Only check the frame content if there's an expected frame
		if i < len(d.frames) {
			assert.Equalf(t, d.frames[i], string(frame), "Reader.ReadFrame frame %d", i)
		}

		// If we should close the reader here, do it and check the expected error
		if c.closeReaderIdx != nil && *c.closeReaderIdx == int64(i) {
			assert.ErrorIsf(t, r.Close(ctx), c.closeReaderErr, "Reader.Close err %d", i)
		}
	}
	assert.Equalf(t, 0, readCloseCounter.count, "Reader should not be closed")
}

type ioWriteCloser struct {
	io.Writer
	io.Closer
}

type recordingCloser struct {
	count int
}

func (c *recordingCloser) Close() error {
	c.count += 1
	return nil
}
