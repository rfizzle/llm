// Package effort is the reasoning level a request asks of the model, the
// one knob every dialect spells differently. A request carries it in
// llm.CompletionOpts.Effort, and each backend fits it to what the model in
// front of it accepts before translating it.
package effort
