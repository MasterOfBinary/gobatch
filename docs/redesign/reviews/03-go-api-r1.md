# Go API review of revision 1

Independent review. Signatures were compiled as stubs. Verdict: changes
to insist on before implementation.

1. Envelope-by-value (`flow.New[Work]()`) compiles and loses writes.
   Consumer-fit then forbade the pointer-envelope workaround. Revision 2
   uses immutable `In` plus owned `any` results instead.
2. Structured errors lack `Error()` / `Unwrap()`; zero `Status` is
   success; `*GraphError` is undefined.
3. `batch.Group` collides with `errgroup.Group`. Rename to `Info`.
   Drop `Attempt` / `Partial`.
4. `Sequence` / `Parallel` / `Map` disagree with Graph on cancel.
   Delete the first two; rename `Map` to `ForEach`.
5. Options contradict “never panic / never silent rewrite”.
6. Duplicate `Event` / `ErrClosed` across packages; plan it.
7. `flow.Run` must stay a package function (no generic methods on 1.25).
8. Default handler timeout 0 (30s invents failure and breaks synctest).
9. Do not attribute conc’s panic-recover to errgroup (reverted).
10. `Loader` godoc must lead with “does not coalesce by key”.
