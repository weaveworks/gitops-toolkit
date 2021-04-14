package serializer

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimetest "k8s.io/apimachinery/pkg/runtime/testing"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const sampleData1 = `# Comment

kind: Test
spec:
  # Head comment
  data:
    - field # Inline comment
    - another
  thing:
    # Head comment
    var: true
`

const sampleData2 = `kind: Test
spec:
  # Head comment
  data:
    - field # Inline comment
    - another:
        subthing: "yes"
  thing:
    # Head comment
    var: true
status:
  nested:
    fields:
    # Just a comment
`

type internalSimpleOM struct {
	runtimetest.InternalSimple
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func withComments(data string) *internalSimpleOM {
	return &internalSimpleOM{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{preserveCommentsAnnotation: data},
		},
	}
}

func parseRNode(t *testing.T, source string) *yaml.RNode {
	rNode, err := yaml.Parse(source)
	if err != nil {
		t.Fatal(err)
	}

	return rNode
}

func TestGetCommentSource(t *testing.T) {
	testCases := []struct {
		name        string
		obj         runtime.Object
		result      string
		expectedErr bool
	}{
		{
			name:        "no_ObjectMeta",
			obj:         &runtimetest.InternalSimple{},
			expectedErr: true,
		},
		{
			name:        "no_comments",
			obj:         &internalSimpleOM{},
			expectedErr: true,
		},
		{
			name:        "invalid_comments",
			obj:         withComments("ä"),
			expectedErr: true,
		},
		{
			name:        "successful_parsing",
			obj:         withComments(base64.StdEncoding.EncodeToString([]byte(sampleData1))),
			result:      sampleData1,
			expectedErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source, actualErr := GetCommentSource(tc.obj)
			if (actualErr != nil) != tc.expectedErr {
				t.Errorf("expected error %t, but received %t: %v", tc.expectedErr, actualErr != nil, actualErr)
			}

			if actualErr != nil {
				// Already handled above.
				return
			}

			str, err := source.String()
			require.NoError(t, err)
			assert.Equal(t, tc.result, str)
		})
	}
}

func TestSetCommentSource(t *testing.T) {
	testCases := []struct {
		name        string
		obj         runtime.Object
		source      *yaml.RNode
		result      string
		expectedErr bool
	}{
		{
			name:        "no_ObjectMeta",
			obj:         &runtimetest.InternalSimple{},
			source:      yaml.NewScalarRNode("test"),
			expectedErr: true,
		},
		{
			name:        "nil_source",
			obj:         &internalSimpleOM{},
			source:      nil,
			result:      "",
			expectedErr: false,
		},
		{
			name:        "successful_parsing",
			obj:         &internalSimpleOM{},
			source:      parseRNode(t, sampleData1),
			result:      sampleData1,
			expectedErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualErr := SetCommentSource(tc.obj, tc.source)
			if (actualErr != nil) != tc.expectedErr {
				t.Errorf("expected error %t, but received %t: %v", tc.expectedErr, actualErr != nil, actualErr)
			}

			if actualErr != nil {
				// Already handled above.
				return
			}

			meta, ok := toMetaObject(tc.obj)
			if !ok {
				t.Fatal("cannot extract metav1.ObjectMeta")
			}

			annotation, ok := getAnnotation(meta, preserveCommentsAnnotation)
			if !ok {
				t.Fatal("expected annotation to be set, but it is not")
			}

			str, err := base64.StdEncoding.DecodeString(annotation)
			require.NoError(t, err)
			assert.Equal(t, tc.result, string(str))
		})
	}
}

func TestCommentSourceSetGet(t *testing.T) {
	testCases := []struct {
		name   string
		source string
	}{
		{
			name:   "encode_decode_1",
			source: sampleData1,
		},
		{
			name:   "encode_decode_2",
			source: sampleData2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			obj := &internalSimpleOM{}
			assert.NoError(t, SetCommentSource(obj, parseRNode(t, tc.source)))

			rNode, err := GetCommentSource(obj)
			assert.NoError(t, err)

			str, err := rNode.String()
			require.NoError(t, err)
			assert.Equal(t, tc.source, str)
		})
	}
}
