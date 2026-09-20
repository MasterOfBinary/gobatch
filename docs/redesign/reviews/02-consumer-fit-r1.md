# Consumer-fit review of revision 1

Independent review against official issues `#71`, `#73`, `#95`, `#97`–`#100`
and ShitQuant house rules. Verdict: placement yes, contracts no.

Must-fix for revision 2:

1. The recommended flow model is the shared mutable analysis object
   `#97` forbade. `#98` wants owned results, immutable views, an
   explicit join.
2. The Decode/Metadata/Prices/Persist example writes `*Work` concurrently
   and puts an unmeasured linger on persist.
3. Batcher abort can hang forever (no budget / no `Wait`).
4. Loader shutdown does not settle waiters on budget expiry (`#71`).
5. Formation and safety are still one knob; formed-not-started groups
   are unbounded (`#73`).
6. Silent `MinItems` clamp contradicts §1 and §4.
7. `Run` panics if called twice (`#95`).
8. Flow `Close` does not implement `#98` shutdown.
9. `#100` linger × executing-node bound is unspecified.
10. Section 8.1 over-claims file-level operational detail the environment
    could not verify.

What to keep: product cut B; path-level Yes/No for ShitQuant; OnyxCore
and shitlock boundaries; no implicit key coalescing; cancel ≠ drain;
missing result ≠ zero `Out`.
