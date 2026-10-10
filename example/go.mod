module github.com/go-fleettools/landingkit/example

go 1.27.1

require github.com/go-fleettools/landingkit v0.0.0

// CI builds this site against the checkout, so a pull request is tested with
// the partials it changes. A landing requires a tagged version instead.
replace github.com/go-fleettools/landingkit => ../
