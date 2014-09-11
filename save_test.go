package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"text/template"
)

// node represents a file tree or a VCS repo
type node struct {
	path    string      // file name or commit type
	body    interface{} // file contents or commit tag
	entries []*node     // nil if the entry is a file
}

var (
	pkgtpl = template.Must(template.New("package").Parse(`package {{.Name}}

import (
{{range .Imports}}	{{printf "%q" .}}
{{end}})
`))
)

func pkg(name string, pkg ...string) string {
	v := struct {
		Name    string
		Imports []string
	}{name, pkg}
	var buf bytes.Buffer
	err := pkgtpl.Execute(&buf, v)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

func decl(name string) string {
	return "var " + name + " int\n"
}

func goderps(importpath string, keyval ...string) *Goderps {
	g := &Goderps{
		ImportPath: importpath,
	}
	for i := 0; i < len(keyval); i += 2 {
		g.Deps = append(g.Deps, Dependency{
			ImportPath: keyval[i],
			Comment:    keyval[i+1],
		})
	}
	return g
}

func TestSave(t *testing.T) {
	var cases = []struct {
		cwd      string
		args     []string
		flagR    bool
		start    []*node
		altstart []*node
		want     []*node
		wdep     Goderps
		werr     bool
	}{
		{
			// dependency on parent directory in same repo
			// see bug https://github.com/tools/godep/issues/70
			cwd:  "P",
			args: []string{"./..."},
			start: []*node{
				{
					"P",
					"",
					[]*node{
						{"main.go", pkg("P"), nil},
						{"Q/main.go", pkg("Q", "P"), nil},
						{"+git", "C1", nil},
					},
				},
			},
			want: []*node{
				{"P/main.go", pkg("P"), nil},
				{"P/Q/main.go", pkg("Q", "P"), nil},
			},
			wdep: Goderps{
				ImportPath: "P",
				Deps:       []Dependency{},
			},
		},
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	const scratch = "goderptest"
	defer os.RemoveAll(scratch)
	for _, test := range cases {
		err = os.RemoveAll(scratch)
		if err != nil {
			t.Fatal(err)
		}
		altsrc := filepath.Join(scratch, "r2", "src")
		if test.altstart != nil {
			makeTree(t, &node{altsrc, "", test.altstart}, "")
		}
		src := filepath.Join(scratch, "r1", "src")
		makeTree(t, &node{src, "", test.start}, altsrc)

		dir := filepath.Join(wd, src, test.cwd)
		err = os.Chdir(dir)
		if err != nil {
			panic(err)
		}
		root1 := filepath.Join(wd, scratch, "r1")
		root2 := filepath.Join(wd, scratch, "r2")
		err = os.Setenv("GOPATH", root1+string(os.PathListSeparator)+root2)
		if err != nil {
			panic(err)
		}
		saveR = test.flagR
		err = save(test.args)
		if g := err != nil; g != test.werr {
			if err != nil {
				t.Log(err)
			}
			t.Errorf("save err = %v want %v", g, test.werr)
		}
		err = os.Chdir(wd)
		if err != nil {
			panic(err)
		}

		checkTree(t, &node{src, "", test.want})

		f, err := os.Open(filepath.Join(dir, "Goderps"))
		if err != nil {
			t.Error(err)
		}
		g := new(Goderps)
		err = json.NewDecoder(f).Decode(g)
		if err != nil {
			t.Error(err)
		}
		f.Close()

		if g.ImportPath != test.wdep.ImportPath {
			t.Errorf("ImportPath = %s want %s", g.ImportPath, test.wdep.ImportPath)
		}
		for i := range g.Deps {
			g.Deps[i].Rev = ""
		}
		if !reflect.DeepEqual(g.Deps, test.wdep.Deps) {
			t.Errorf("Deps = %v want %v", g.Deps, test.wdep.Deps)
		}
	}
}

func makeTree(t *testing.T, tree *node, altpath string) (gopath string) {
	walkTree(tree, tree.path, func(path string, n *node) {
		g, isGoderps := n.body.(*Goderps)
		body, _ := n.body.(string)
		switch {
		case isGoderps:
			for i, dep := range g.Deps {
				rel := filepath.FromSlash(dep.ImportPath)
				dir := filepath.Join(tree.path, rel)
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					dir = filepath.Join(altpath, rel)
				}
				tag := dep.Comment
				rev := strings.TrimSpace(run(t, dir, "git", "rev-parse", tag))
				g.Deps[i].Rev = rev
			}
			os.MkdirAll(filepath.Dir(path), 0770)
			f, err := os.Create(path)
			if err != nil {
				t.Errorf("makeTree: %v", err)
				return
			}
			defer f.Close()
			err = json.NewEncoder(f).Encode(g)
			if err != nil {
				t.Errorf("makeTree: %v", err)
			}
		case n.path == "+git":
			dir := filepath.Dir(path)
			run(t, dir, "git", "init") // repo might already exist, but ok
			run(t, dir, "git", "add", ".")
			run(t, dir, "git", "commit", "-m", "goderp")
			if body != "" {
				run(t, dir, "git", "tag", body)
			}
		case n.entries == nil && strings.HasPrefix(body, "symlink:"):
			target := strings.TrimPrefix(body, "symlink:")
			os.Symlink(target, path)
		case n.entries == nil && body == "(absent)":
			panic("is this gonna be forever")
		case n.entries == nil:
			os.MkdirAll(filepath.Dir(path), 0770)
			err := ioutil.WriteFile(path, []byte(body), 0660)
			if err != nil {
				t.Errorf("makeTree: %v", err)
			}
		default:
			os.MkdirAll(path, 0770)
		}
	})
	return gopath
}

func checkTree(t *testing.T, want *node) {
	walkTree(want, want.path, func(path string, n *node) {
		body := n.body.(string)
		switch {
		case n.path == "+git":
			panic("is this real life")
		case n.entries == nil && strings.HasPrefix(body, "symlink:"):
			panic("why is this happening to me")
		case n.entries == nil && body == "(absent)":
			body, err := ioutil.ReadFile(path)
			if !os.IsNotExist(err) {
				t.Errorf("checkTree: %s = %s want absent", path, string(body))
				return
			}
		case n.entries == nil:
			gbody, err := ioutil.ReadFile(path)
			if err != nil {
				t.Errorf("checkTree: %v", err)
				return
			}
			if got := string(gbody); got != body {
				t.Errorf("%s = %s want %s", path, got, body)
			}
		default:
			os.MkdirAll(path, 0770)
		}
	})
}

func walkTree(n *node, path string, f func(path string, n *node)) {
	f(path, n)
	for _, e := range n.entries {
		walkTree(e, filepath.Join(path, filepath.FromSlash(e.path)), f)
	}
}

func run(t *testing.T, dir, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic(name + " " + strings.Join(args, " ") + ": " + err.Error())
	}
	return string(out)
}
