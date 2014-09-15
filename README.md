### Goderp

Command goderp helps build packages reproducibly by fixing their
dependencies.

This tool assumes you are working in a standard Go workspace, as
described in http://golang.org/doc/code.html. We require Go 1.1 or
newer to build goderp itself, but you can use it on any project
that works with Go 1 or newer.

### Install

``` bash
go get github.com/meatballhat/goderp
```

### Migrating from godep

``` bash
git mv Godep Goderp
```
	
#### Getting Started

How to add goderp in a new project.

Assuming you've got everything working already, so you can build
your project with `go install` and test it with `go test`, it's
one command to start using:

``` bash
goderp save
```

This will save a list of dependencies to the file `Goderps`, Read
over its contents and make sure it looks reasonable.  Then commit
the file to version control.

#### Restore

The `goderp restore` command is the opposite of `goderp save`.  It
will install the package versions specified in `Goderps` to your
`$GOPATH`.

#### Edit-test Cycle

1. Edit code
2. Run `goderp go test`
3. (repeat)

#### Add a Dependency

To add a new package foo/bar, do this:

1. Run `go get foo/bar`
2. Edit your code to import foo/bar.
3. Run `goderp save` (or `goderp save ./...`).

#### Update a Dependency

To update a package from your `$GOPATH`, do this:

1. Run `go get -u foo/bar`
2. Run `goderp update foo/bar`. (You can use the `...` wildcard,
for example `goderp update foo/...`).

Before committing the change, you'll probably want to inspect
the changes to `Goderps`, for example with `git diff`,
and make sure it looks reasonable.

#### Multiple Packages

If your repository has more than one package, you're probably
accustomed to running commands like `go test ./...`,
`go install ./...`, and `go fmt ./...`.
Similarly, you should run `goderp save ./...` to capture the
dependencies of all packages.

#### Using Other Tools

The `goderp path` command helps integrate with commands other than
the standard go tool. This works with any tool that reads `$GOPATH`
from its environment, for example the recently-released [oracle
command](http://godoc.org/code.google.com/p/go.tools/cmd/oracle).

``` bash
GOPATH=`goderp path`:$GOPATH
oracle -mode=implements .
```

### File Format

`Goderps` is a json file with the following structure:

```go
type Goderps struct {
	ImportPath string
	GoVersion  string   // Abridged output of 'go version'.
	Packages   []string // Arguments to goderp save, if any.
	Deps       []struct {
		ImportPath string
		Comment    string // Description of commit, if present.
		Rev        string // VCS-specific commit ID.
	}
}
```

Example `Goderps`:

```json
{
    "ImportPath": "github.com/kr/hk",
    "GoVersion": "go1.1.2",
    "Deps": [
        {
            "ImportPath": "code.google.com/p/go-netrc/netrc",
            "Rev": "28676070ab99"
        },
        {
            "ImportPath": "github.com/kr/binarydist",
            "Rev": "3380ade90f8b0dfa3e363fd7d7e941fa857d0d13"
        }
    ]
}
```

### Plagiarism Alert

Goderp is a fork of [Godep](github.com/tools/godep), and makes no
attempt to hide it.  Take a look at the repo history.  It's all
there.  The code fork is a reflection of the philosophical fork
that happened in the Godep project when `save -copy=false` was
deprecated and slated for removal.  Goderp chooses the other path,
making `save -copy=false` the default behavior and `save
-copy=true` into a no-op.
