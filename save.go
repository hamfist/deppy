package main

import (
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kr/fs"
)

var cmdSave = &Command{
	Usage: "save [-r] [-copy=false] [packages]",
	Short: "list and copy dependencies into Deps",
	Long: `
Save writes a list of the dependencies of the named packages along
with the exact source control revision of each dependency to a file
named "Deps".

The dependency list is a JSON document with the following structure:

	type Deps struct {
		ImportPath string
		GoVersion  string   // Abridged output of 'go version'.
		Packages   []string // Arguments to deppy save, if any.
		Deps       []struct {
			ImportPath string
			Comment    string // Tag or description of commit.
			Rev        string // VCS-specific commit ID.
		}
	}

Any dependencies already present in the list will be left unchanged.

For more about specifying packages, see 'go help packages'.
`,
	Run: runSave,
}

var (
	saveCopy = true
)

func init() {
	cmdSave.Flag.BoolVar(&saveCopy, "copy", false, "copy source code")
}

func runSave(cmd *Command, args []string) {
	err := save(args)
	if err != nil {
		log.Fatalln(err)
	}
}

func save(pkgs []string) error {
	if saveCopy {
		log.Println(strings.TrimSpace(copyWarning))
	}
	dot, err := LoadPackages(".")
	if err != nil {
		return err
	}
	ver, err := goVersion()
	if err != nil {
		return err
	}
	manifest := "Deps"
	var gold Deps
	gnew := &Deps{
		ImportPath: dot[0].ImportPath,
		GoVersion:  ver,
	}
	if len(pkgs) > 0 {
		gnew.Packages = pkgs
	} else {
		pkgs = []string{"."}
	}
	a, err := LoadPackages(pkgs...)
	if err != nil {
		return err
	}
	err = gnew.Load(a)
	if err != nil {
		return err
	}
	if a := badSandboxVCS(gnew.Deps); a != nil {
		log.Println("Unsupported sandbox VCS:", strings.Join(a, ", "))
		return errors.New("error")
	}
	if gnew.Deps == nil {
		gnew.Deps = make([]Dependency, 0) // produce json [], not null
	}
	err = carryVersions(&gold, gnew)
	if err != nil {
		return err
	}
	err = os.RemoveAll("Deps")
	if err != nil {
		log.Println(err)
	}
	f, err := os.Create(manifest)
	if err != nil {
		return err
	}
	_, err = gnew.WriteTo(f)
	if err != nil {
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}
	var rewritePaths []string
	return rewrite(a, dot[0].ImportPath, rewritePaths)
}

type revError struct {
	ImportPath string
	HaveRev    string
	WantRev    string
}

func (v *revError) Error() string {
	return v.ImportPath + ": revision is " + v.HaveRev + ", want " + v.WantRev
}

// carryVersions copies Rev and Comment from a to b for
// each dependency with an identical ImportPath. For any
// dependency in b that appears to be from the same repo
// as one in a (for example, a parent or child directory),
// the Rev must already match - otherwise it is an error.
func carryVersions(a, b *Deps) error {
	for i := range b.Deps {
		err := carryVersion(a, &b.Deps[i])
		if err != nil {
			return err
		}
	}
	return nil
}

func carryVersion(a *Deps, db *Dependency) error {
	// First see if this exact package is already in the list.
	for _, da := range a.Deps {
		if db.ImportPath == da.ImportPath {
			db.Rev = da.Rev
			db.Comment = da.Comment
			return nil
		}
	}
	// No exact match, check for child or sibling package.
	// We can't handle mismatched versions for packages in
	// the same repo, so report that as an error.
	for _, da := range a.Deps {
		switch {
		case strings.HasPrefix(db.ImportPath, da.ImportPath+"/"):
			if da.Rev != db.Rev {
				return &revError{db.ImportPath, db.Rev, da.Rev}
			}
		case strings.HasPrefix(da.ImportPath, db.root+"/"):
			if da.Rev != db.Rev {
				return &revError{db.ImportPath, db.Rev, da.Rev}
			}
		}
	}
	// No related package in the list, must be a new repo.
	return nil
}

// subDeps returns a - b, using ImportPath for equality.
func subDeps(a, b []Dependency) (diff []Dependency) {
Diff:
	for _, da := range a {
		for _, db := range b {
			if da.ImportPath == db.ImportPath {
				continue Diff
			}
		}
		diff = append(diff, da)
	}
	return diff
}

// badSandboxVCS returns a list of VCSes that don't work
// with the `deppy go` sandbox code.
func badSandboxVCS(deps []Dependency) (a []string) {
	for _, d := range deps {
		if d.vcs.CreateCmd == "" {
			a = append(a, d.vcs.vcs.Name)
		}
	}
	sort.Strings(a)
	return uniq(a)
}

func removeSrc(srcdir string, deps []Dependency) error {
	for _, dep := range deps {
		path := filepath.FromSlash(dep.ImportPath)
		err := os.RemoveAll(filepath.Join(srcdir, path))
		if err != nil {
			return err
		}
	}
	return nil
}

func copySrc(dir string, deps []Dependency) error {
	ok := true
	for _, dep := range deps {
		srcdir := filepath.Join(dep.ws, "src")
		rel, err := filepath.Rel(srcdir, dep.dir)
		if err != nil { // this should never happen
			return err
		}
		dstpkgroot := filepath.Join(dir, rel)
		err = os.RemoveAll(dstpkgroot)
		if err != nil {
			log.Println(err)
			ok = false
		}
		w := fs.Walk(dep.dir)
		for w.Step() {
			err = copyPkgFile(dir, srcdir, w)
			if err != nil {
				log.Println(err)
				ok = false
			}
		}
	}
	if !ok {
		return errors.New("error copying source code")
	}
	return nil
}

func copyPkgFile(dstroot, srcroot string, w *fs.Walker) error {
	if w.Err() != nil {
		return w.Err()
	}
	if c := w.Stat().Name()[0]; c == '.' || c == '_' {
		// Skip directories using a rule similar to how
		// the go tool enumerates packages.
		// See $GOROOT/src/cmd/go/main.go:/matchPackagesInFs
		w.SkipDir()
	}
	if w.Stat().IsDir() {
		return nil
	}
	rel, err := filepath.Rel(srcroot, w.Path())
	if err != nil { // this should never happen
		return err
	}
	return copyFile(filepath.Join(dstroot, rel), w.Path())
}

// copyFile copies a regular file from src to dst.
// dst is opened with os.Create.
func copyFile(dst, src string) error {
	err := os.MkdirAll(filepath.Dir(dst), 0777)
	if err != nil {
		return err
	}

	linkDst, err := os.Readlink(src)
	if err == nil {
		return os.Symlink(linkDst, dst)
	}

	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()

	w, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, r)
	err1 := w.Close()
	if err == nil {
		err = err1
	}

	return err
}

// Func writeVCSIgnore writes "ignore" files inside dir for known VCSs,
// so that dir/pkg and dir/bin don't accidentally get committed.
// It logs any errors it encounters.
func writeVCSIgnore(dir string) {
	// Currently git is the only VCS for which we know how to do this.
	// Mercurial and Bazaar have similar mechasims, but they apparently
	// require writing files outside of dir.
	const ignore = "/pkg\n/bin\n"
	name := filepath.Join(dir, ".gitignore")
	err := writeFile(name, ignore)
	if err != nil {
		log.Println(err)
	}
}

// writeFile is like ioutil.WriteFile but it creates
// intermediate directories with os.MkdirAll.
func writeFile(name, body string) error {
	err := os.MkdirAll(filepath.Dir(name), 0777)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(name, []byte(body), 0666)
}

const (
	copyWarning = `
deprecated flag -copy=true

The flag -copy=true does not exist.  It's just gone.  Wow!
`
)
