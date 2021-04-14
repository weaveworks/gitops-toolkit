package git

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fluxcd/go-git-providers/gitprovider"
)

type filesChangedSubTest struct {
	fromCommit string
	toCommit   string
	want       []string
	wantErr    bool
}

type readFileSubTest struct {
	commit  string
	file    string
	wantErr bool
}

func Test_goGit(t *testing.T) {
	tests := []struct {
		name         string
		repoRef      string
		opts         []Option
		filesChanged []filesChangedSubTest
		readFiles    []readFileSubTest
	}{
		{
			name:    "default",
			repoRef: "https://github.com/weaveworks/libgitops",
			filesChanged: []filesChangedSubTest{
				{
					fromCommit: "5843c185b995e566fe245f7abb27f4c8cffcae71",
					toCommit:   "2e1789bf3be4cf03eb3b5b7d778f8cd6c39d40c7",
					want: []string{
						"pkg/storage/transaction/git.go",
						"pkg/storage/transaction/pullrequest/github/github.go",
						"pkg/util/util.go",
					},
				},
				{
					fromCommit: "5843c185b995e566fe245f7abb27f4c8cffcae71",
					toCommit:   "5843c185b995e566fe245f7abb27f4c8cffcae71",
					want:       []string{"pkg/storage/transaction/pullrequest/github/github.go"},
				},
			},
			readFiles: []readFileSubTest{
				{
					commit: "19bdfaa92ba594b9d16312e7c923ff9ef09c65d7",
					file:   "README.md",
				},
				{
					commit: "fb15f0063ff486debbf525c460797b144c5d641f",
					file:   "README.md",
				},
			},
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("repo_%d", i), func(t *testing.T) {
			d, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(d)
			ctx := context.Background()
			repoRef, err := gitprovider.ParseOrgRepositoryURL(tt.repoRef)
			if err != nil {
				t.Fatal(err)
			}
			g, err := NewGoGit(ctx, repoRef, d, defaultOpts().ApplyOptions(tt.opts))
			if err != nil {
				t.Fatal(err)
			}
			Subtest_filesChanged(t, g, tt.filesChanged)
			Subtest_readFiles(t, g, tt.readFiles)
		})
	}
}

func Subtest_filesChanged(t *testing.T, g Interface, tests []filesChangedSubTest) {
	ctx := context.Background()
	for i, tt := range tests {
		t.Run(fmt.Sprintf("filesChanged_%d", i), func(t *testing.T) {
			got, err := g.FilesChanged(ctx, tt.fromCommit, tt.toCommit)
			if (err != nil) != tt.wantErr {
				t.Errorf("goGit.FilesChanged() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.List(), tt.want) {
				t.Errorf("goGit.FilesChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Subtest_readFiles(t *testing.T, g Interface, tests []readFileSubTest) {
	ctx := context.Background()
	for i, tt := range tests {
		t.Run(fmt.Sprintf("readFiles_%d", i), func(t *testing.T) {
			got, err := g.ReadFileAtCommit(ctx, tt.commit, tt.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("goGit.ReadFileAtCommit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			validateFile := fmt.Sprintf("testdata/%s_%s", tt.commit, strings.ReplaceAll(tt.file, "/", "_"))
			want, err := ioutil.ReadFile(validateFile)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("goGit.ReadFileAtCommit() = %v, want %v", got, want)
			}
		})
	}
}
