package llm

import "github.com/rfizzle/llm/internal/idle"

// DefaultStreamIdle and ErrStreamIdle are the deadline every dialect's stream
// runs under (internal/idle), named here for the apps that read them.
//
// DefaultStreamIdle is how long a stream may go without an event before the
// turn fails. ErrStreamIdle is the failure a stream that went quiet ends on.
const DefaultStreamIdle = idle.Default

var ErrStreamIdle = idle.ErrStreamIdle
