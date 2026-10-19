// Package flow names the bounded calls an app makes outside its main agent
// and their children: a classifier verdict, a title, a summary. A request
// carries its flow in llm.CompletionOpts.Flow, set by the code that builds
// it, and a profile's declaration scoped to that flow narrows that request
// and no other.
package flow
