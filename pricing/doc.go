// Package pricing is the model data: what a model costs, how much context
// it has, and how it spells its thinking level. One public table carries all
// three, so one download serves the spend meter, the context gauge, and the
// reasoning ladder — and one snapshot of it ships inside the binary as the
// floor under the download.
//
// It prices a request and reports whether it found the model; it keeps no
// budget, and what an unpriced request means is the caller's to decide.
package pricing
