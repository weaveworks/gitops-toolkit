package filesystem

import (
	"testing"
)

func TestExcludeGitDirectory_ShouldExcludePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "normal",
			path: ".git/foo",
			want: true,
		},
		{
			name: "with relative path",
			path: "./.git/bar/baz",
			want: true,
		},
		{
			name: "with many parents",
			path: "/foo/bar/.git/hello",
			want: true,
		},
		{
			name: "with many children",
			path: ".git/foo/bar/baz",
			want: true,
		},
		{
			name: "with parents and children",
			path: "./foo/bar/.git/baz/bar",
			want: true,
		},
		{
			name: "empty",
			path: "",
			want: false,
		},
		{
			name: "local dir",
			path: ".",
			want: false,
		},
		{
			name: "other prefix",
			path: "foo.git",
			want: false,
		},
		{
			name: "other suffix",
			path: ".gitea",
			want: false,
		},
		{
			name: "absolute path without git",
			path: "/foo/bar/no/git/here",
			want: false,
		},
		{
			name: "don't catch files named .git",
			path: "/hello/.git",
			want: false,
		},
	}
	e := ExcludeDirectoryNames{DirectoryNamesToExclude: []string{".git"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.ShouldExcludePath(tt.path); got != tt.want {
				t.Errorf("ExcludeGitDirectory.ShouldExcludePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
