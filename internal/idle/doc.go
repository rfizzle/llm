// Package idle is the idle deadline every turn's stream runs under.
//
// A request that is accepted, answered with headers and then never written to
// again is indistinguishable, from inside the loop, from a model that is
// thinking hard: both are a blocking read that has not returned. An
// interactive caller has a person to cancel it; an unattended one has nobody,
// and holds whatever it holds until something external kills it.
//
// So every dialect's stream loop pushes a deadline forward on each event it
// reads, and a deadline that expires cancels the request and ends the turn as
// a network failure — which is a class the caller's retry schedule already
// waits out and asks again.
package idle
