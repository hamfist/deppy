/*

Command goderp helps build packages reproducibly by fixing
their dependencies.

Example Usage

Save currently-used dependencies to file Goderps:

	$ goderp save

Build project using saved dependencies:

	$ goderp go install

or

	$ GOPATH=`goderp path`:$GOPATH
	$ go install

*/
package main
