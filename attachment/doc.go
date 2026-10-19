// Package attachment holds the parts of a message that are not
// conversation: a pasted picture, a file, a recording. A message carries
// them in llm.Message.Attachments, and each backend decides how to put one on
// the wire; only the fallback text form is shared.
package attachment
