package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnqualify(t *testing.T) {
	var cases = []struct {
		path string
		want string
	}{
		{"C", "C"},
		{"D/Deps/_workspace/src/T", "T"},
		{"C/Deps/_workspace/src/D/Deps/_workspace/src/T", "T"},
	}
	for _, test := range cases {
		g := unqualify(test.path)
		if g != test.want {
			t.Errorf("qualify(%s) = %s want %s", test.path, g, test.want)
		}
	}
}

func TestQualify(t *testing.T) {
	var cases = []struct {
		path string
		want string
	}{
		{"C", "C"},
		{"C/P", "C/P"},
		{"fmt", "fmt"},
		{"DP", "DP"},
		{"D", "C/Deps/_workspace/src/D"},
		{"D/P", "C/Deps/_workspace/src/D/P"},
	}
	for _, test := range cases {
		g := qualify(test.path, "C", []string{"D"})
		if g != test.want {
			t.Errorf("qualify({C}, %s) = %s want %s", test.path, g, test.want)
		}
	}
}

const (
	whitespace = `package main

import "D"

var (
	x   int
	abc int
)
`
	whitespaceRewritten = `package main

import "C/Deps/_workspace/src/D"

var (
	x   int
	abc int
)
`
)

func TestRewrite(t *testing.T) {
	var cases = []struct {
		cwd   string
		paths []string
		start []*node
		want  []*node
		werr  bool
	}{
		{ // simple case, one dependency
			cwd:   "C",
			paths: []string{"D"},
			start: []*node{
				{"C/main.go", pkg("main", "D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D"), nil},
			},
			want: []*node{
				{"C/main.go", pkg("main", "C/Deps/_workspace/src/D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D"), nil},
			},
		},
		{ // transitive dep
			cwd:   "C",
			paths: []string{"D", "T"},
			start: []*node{
				{"C/main.go", pkg("main", "D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D", "T"), nil},
				{"C/Deps/_workspace/src/T/main.go", pkg("T"), nil},
			},
			want: []*node{
				{"C/main.go", pkg("main", "C/Deps/_workspace/src/D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D", "C/Deps/_workspace/src/T"), nil},
				{"C/Deps/_workspace/src/T/main.go", pkg("T"), nil},
			},
		},
		{ // intermediate dep that uses goderp save -r
			cwd:   "C",
			paths: []string{"D", "T"},
			start: []*node{
				{"C/main.go", pkg("main", "D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D", "D/Deps/_workspace/src/T"), nil},
				{"C/Deps/_workspace/src/T/main.go", pkg("T"), nil},
			},
			want: []*node{
				{"C/main.go", pkg("main", "C/Deps/_workspace/src/D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D", "C/Deps/_workspace/src/T"), nil},
				{"C/Deps/_workspace/src/T/main.go", pkg("T"), nil},
			},
		},
		{ // don't qualify standard library and local imports
			cwd: "C",
			start: []*node{
				{"C/main.go", pkg("main", "fmt", "C/D"), nil},
				{"C/D/main.go", pkg("D"), nil},
			},
			want: []*node{
				{"C/main.go", pkg("main", "fmt", "C/D"), nil},
				{"C/D/main.go", pkg("D"), nil},
			},
		},
		{ // simple case, one dependency, -r=false
			cwd: "C",
			start: []*node{
				{"C/main.go", pkg("main", "D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D"), nil},
			},
			want: []*node{
				{"C/main.go", pkg("main", "D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D"), nil},
			},
		},
		{ // transitive dep, -r=false
			cwd: "C",
			start: []*node{
				{"C/main.go", pkg("main", "D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D", "T"), nil},
				{"C/Deps/_workspace/src/T/main.go", pkg("T"), nil},
			},
			want: []*node{
				{"C/main.go", pkg("main", "D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D", "T"), nil},
				{"C/Deps/_workspace/src/T/main.go", pkg("T"), nil},
			},
		},
		{ // intermediate dep that uses goderp save -r, -r=false
			cwd: "C",
			start: []*node{
				{"C/main.go", pkg("main", "D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D", "D/Deps/_workspace/src/T"), nil},
				{"C/Deps/_workspace/src/T/main.go", pkg("T"), nil},
			},
			want: []*node{
				{"C/main.go", pkg("main", "D"), nil},
				{"C/Deps/_workspace/src/D/main.go", pkg("D", "T"), nil},
				{"C/Deps/_workspace/src/T/main.go", pkg("T"), nil},
			},
		},
		{ // whitespace
			cwd:   "C",
			paths: []string{"D"},
			start: []*node{
				{"C/main.go", whitespace, nil},
			},
			want: []*node{
				{"C/main.go", whitespaceRewritten, nil},
			},
		},
	}

	const gopath = "goderptest"
	defer os.RemoveAll(gopath)
	for _, test := range cases {
		err := os.RemoveAll(gopath)
		if err != nil {
			t.Fatal(err)
		}
		src := filepath.Join(gopath, "src")
		makeTree(t, &node{src, "", test.start}, "")
		err = rewriteTree(filepath.Join(src, test.cwd), test.cwd, test.paths)
		if g := err != nil; g != test.werr {
			t.Errorf("save err = %v (%v) want %v", g, err, test.werr)
		}

		checkTree(t, &node{src, "", test.want})
	}
}
