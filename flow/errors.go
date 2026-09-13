package flow

import (
	"errors"

	"github.com/MasterOfBinary/gobatch/internal/panicguard"
)

// ErrNilStep reports an explicitly declared nil step. A nil branch does not
// prevent other Parallel branches from starting and joining.
var ErrNilStep = errors.New("flow: nil step")

// PanicError reports a recovered step panic.
// Error text contains only safe boundary metadata. Stack returns a defensive
// copy of protected diagnostics capped at 16 KiB; Truncated marks a byte-limit cut.
// The panic value is neither retained nor formatted. Recovery is not rollback.
type PanicError = panicguard.PanicError
