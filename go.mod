module github.com/go-fleettools/landingkit

// ⛔ 1.27.1 left EIGHT advisories reachable from this module -- not merely
// present: GO-2026-6605 through stdlib.Get, 6613 through Read, 6611 and 6607
// through Write, 6603 through Close, 6610 and 6617 through AddConn, 6608
// through ReadMIMEHeader. This module fetches pages, so those are its own HTTP
// paths, and every one of them is fixed in 1.27.2.
//
// And this line IS the lane's toolchain: ci.yml reads `go-version-file:
// go.mod`, so a directive below the fix is a lane linking the vulnerable
// standard library, not just a floor for consumers.
go 1.27.3
