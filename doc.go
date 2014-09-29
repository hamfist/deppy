/*

Command deppy helps build packages reproducibly by fixing
their dependencies.

Example Usage

Save currently-used dependencies to file Deps:

	$ deppy save

Build project using saved dependencies:

	$ deppy go install

or

	$ GOPATH=`deppy path`:$GOPATH
	$ go install

*/
package main
