package batch

// Logger is a minimal logging interface used by GoBatch to emit diagnostic
// information.  It is intentionally kept compatible with the standard library
// log.Logger as well as many popular structured loggers that expose a
// `Printf`-style method (e.g. slog.Logger, zap.SugaredLogger).
//
// Only a single Printf-style method is required so that a wide variety of
// logger implementations can be plugged in without introducing any additional
// dependencies into GoBatch itself.
//
// If you need richer logging (Infof, Errorf, With, etc.) you can satisfy the
// interface by forwarding all calls to the appropriate log-level in your own
// logger.
//
// Keeping the interface minimal guarantees full backwards-compatibility: users
// that do not care about logging do not need to make any code changes – a
// silent no-op logger is used by default.
//
//
//  b := batch.New(cfg).WithLogger(log.Default())
//  errs := b.Go(ctx, src, proc) // all internal errors are now logged via the
//  // provided logger
//
// The logger is completely optional; if it isn't supplied, GoBatch continues to
// behave exactly as before.
//
// NOTE: The interface lives in the batch package to avoid import cycles and to
// make it easily discoverable by users.
type Logger interface {
    // Printf logs a formatted message.
    // The semantics are identical to log.Printf.
    Printf(format string, v ...interface{})
}

// noopLogger is used when the user hasn't provided a custom Logger. It discards
// all log messages so that logging calls have zero overhead in that scenario.
type noopLogger struct{}

func (n *noopLogger) Printf(_ string, _ ...interface{}) {}