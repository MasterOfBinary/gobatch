# Ultra-Brainstorm Report — gobatch-10-projects (2026-06-12)

Run: `20260612T061517Z-gobatch-10-projects` · Scratch: `.ultra-brainstorm/20260612T061517Z-gobatch-10-projects/`

## Idea Brief

**What**: A slate of 14 candidate personal software projects, each meaningfully built on gobatch (`github.com/MasterOfBinary/gobatch`), Vaughn's own Go micro-batching library. Goal: harden and cull to the 10 best. Most run unattended in the cloud for personal use; a few may be sellable if natural.

**gobatch capabilities**: Generic `Batch[T]` pipeline: `Source[T]` (e.g. channels) → batches formed by min/max item count and time windows (static or runtime-adjustable config) → `Processor[T]` chains (Transform, Filter, Channel out). Per-item IDs and error tracking; processing continues despite item errors. v0, API unstable, single maintainer = the builder himself. Sweet spot: turning streams of small events into efficient bulk operations (bulk DB writes, rate-limited API calls, periodic aggregation). Not a scheduler, queue, or workflow engine.

**Builder persona**: Solo dev "MasterOfBinary," scrappy visionary, professionally chaotic. Loves Go, Python, TypeScript/Next.js, GCP (Firestore, GKE), Vercel, LLM APIs, Slack/Discord bots, Stable Diffusion/Imagen. Interests: mental wellness/microjournaling, DJing & music production (Serato), fitness (GVT, 2.4 km runs), Toastmasters, AI comics, gaming (Elden Ring, Monster Hunter). Values joy, whimsy, shipping fast; avoids corporate formality and meaningless AI integrations. Wants projects that stay fun, not chores.

**Candidates (cull/replace/keep → final 10)**: 1. mood-journal · 2. dj-library-intel · 3. workout-logger · 4. llm-micro-batcher · 5. comic-render-queue · 6. toastmasters-ops · 7. discord-meme-digest · 8. webhook-fanin-hub · 9. new-music-watcher · 10. photo-organizer · 11. game-session-stats · 12. gear-price-watcher · 13. error-digester · 14. email-triage

**Constraints**: solo, evenings/weekends; cloud-run (GCP/Vercel) on cheap/free tiers; each must genuinely exercise gobatch's batching (count/time windows), not just name-drop it; personal-use first, sell only if natural.

## MUST-RESOLVE (26 clusters, ≥2 independent lenses, rank order)

### [UB-C1] gear-price-watcher: solved by incumbents, rest is a scraping treadmill
- **Severity**: major · **Lenses**: skeptic, operator, user-advocate, estimator, ecosystem, cost, compliance
- **Summary**: Duplicates free incumbents (Keepa/CamelCamelCamel, eBay saved-search alerts, Reverb's native watchlist); coverage beyond them means scraping anti-bot retail marketplaces from datacenter IPs — a ToS-breaching, ban-prone maintenance treadmill whose failures are ambiguous ("no results" looks identical to "no deals"), so the watcher silently goes stale and creates false confidence. Paid scraping infra (~$30–50/mo) exceeds hobby value outright.
- **Would resolve**: Restrict to sanctioned free APIs/feeds (eBay Browse API, Reverb API, RSS) and verify coverage; name one concrete watch incumbents cannot express; instrument fetch status separately from matches with staleness stamps; if real sources require scraping protected retailers, cull.

### [UB-C2] email-triage: Gmail restricted-scope OAuth breaks "unattended" by design
- **Severity**: major · **Lenses**: operator, historian, estimator, ecosystem, compliance, decomposer
- **Summary**: Unverified consumer-Gmail apps in testing mode get refresh tokens that die ~weekly (service silently stops); publishing requires restricted-scope verification incl. CASA-grade security assessment unrealistic for a solo hobby app; Gmail watch/push adds Pub/Sub renewal infrastructure gobatch does nothing for. The canonical graveyard for unattended personal Gmail tools — a premise-level constraint.
- **Would resolve**: Pick a survivable auth architecture before the candidate can enter the final 10 (IMAP app password, Workspace internal app, auto-forwarding, or accepted weekly re-auth ritual) and prove it with a 30-day unattended token spike; read-only single-user v1 with a daily heartbeat.

### [UB-C3] game-session-stats: the data source does not exist
- **Severity**: **blocker** · **Lenses**: skeptic, user-advocate, estimator, ecosystem, decomposer
- **Summary**: Elden Ring and Monster Hunter emit no session logs: no API, Steam exposes only coarse playtime, consoles offer nothing programmatic, and PC capture means save-file reverse engineering, overlays, or memory reading that Easy Anti-Cheat treats as ban-worthy on the account he actually plays. The acquisition layer is an unsolved local-agent sub-project larger than the stats page, contradicts the cloud-unattended constraint; manual entry turns play into a logging chore. As stated, cannot be built.
- **Would resolve**: A spike proving one safe end-to-end capture path on his platform before it can occupy a slot; absent that, cull.

### [UB-C4] Slate-wide: micro-batching is decorative at personal event volumes
- **Severity**: major · **Lenses**: skeptic, user-advocate, historian, decomposer, cost
- **Summary**: At 1–50 events/day, count/time windows close nearly empty; the "must genuinely exercise gobatch" constraint forces always-on streaming ceremony onto naturally cron-shaped jobs. Poll-then-process candidates already hold the full list in memory so batching degenerates to a for-loop wrapper; gobatch has never shipped a polling/ticker source; claimed cost savings are phantom on per-op-billed backends. The constraint optimizes for the library instead of the user and produces fake dogfooding signal.
- **Would resolve**: For each finalist, one written line: expected events/day, the concrete batching benefit (rate-limit coalescing, round-trip reduction, digest/context chunking — never dollars on per-op backends), and where batching occurs; reframe cron-shaped candidates as scheduled jobs whose internal fan-out honestly uses gobatch.

### [UB-C5] llm-micro-batcher: weak personal-scale premise, "sellable" unrealistic
- **Severity**: major · **Lenses**: skeptic, user-advocate, cost, ecosystem, estimator
- **Summary**: Presumes co-occurring LLM traffic a hobbyist doesn't generate (windows close with a batch of one); client-side coalescing of synchronous calls doesn't reduce per-token price — real ~50% savings live in provider async batch APIs, and embedding endpoints already accept arrays. Free incumbents (LiteLLM, OpenRouter, Helicone, native batch endpoints) own every plausible buyer; the sellable version shares almost no code with the personal one.
- **Would resolve**: Instrument a week of real traffic; keep only if batching demonstrably saves money or unblocks rate limits (bulk embedding backfills are the plausible workload); reframe personal-only with the value pinned to a real mechanism; drop sellable framing until an external user validates a written gap.

### [UB-C6] Fleet on v0 gobatch: every breaking change multiplies by ten
- **Severity**: major · **Lenses**: operator, historian, decomposer, estimator
- **Summary**: All finalists would depend on a v0 library whose maintainer intentionally breaks compatibility on master (latest commit breaks the core Go/New surface; more stacked behind unlanded PRs) and who has twice abandoned the project for multi-year stretches. Each API change fans out as forced migration across up to ten deployed services, or the fleet pins stale snapshots and drifts apart — multiplying the cost of instability the builder himself creates.
- **Would resolve**: Decide dependency policy before fan-out: tag releases and pin every project to a tag; freeze the core v0.x surface or route all projects through one shared adapter module absorbing breakage in one place; batch upgrades as scheduled fleet passes; forbid candidates whose core feature depends on future gobatch work.

### [UB-C7] In-memory batch windows silently lose data; persist-before-batch needed
- **Severity**: major · **Lenses**: operator, estimator, decomposer, historian
- **Summary**: gobatch buffers items in memory with no persistence, retry, or scheduler; a Batch is single-use; any restart, redeploy, or scale-down drops the in-flight window. For human-typed mood/workout entries that is permanent, undetectable loss that destroys trust — and gobatch's own bug history (trailing items under MinItems, MaxTime timer breakage after idle, Filter dropping errored items) clusters exactly in this low-volume/idle regime. Daily/weekly cadence is scheduling plus durable accumulation, both outside the library's scope.
- **Would resolve**: One slate-wide reference architecture: persist raw events durably at ingest (Firestore/Pub/Sub) before or alongside batching; cap gobatch windows at seconds-to-minutes; Cloud Scheduler owns cadence, re-streaming stored rows through gobatch; SIGTERM graceful drain; idle-soak loss test in gobatch CI.

### [UB-C8] photo-organizer: source closed, problem already solved, costs unpriced
- **Severity**: major · **Lenses**: skeptic, estimator, ecosystem, cost
- **Summary**: The natural source is structurally closed (post-2025 Google Photos API changes block reading a user's existing library); the local-files version is already solved free (Immich/PhotoPrism, native Google Photos search/dedupe); the workload is a one-shot backfill plus a trickle, not a stream; the backfill bill swings 100x+ by API choice with accidental re-runs unbounded.
- **Would resolve**: Pin where photos actually live with confirmed unattended access; name a weekly-felt behavior incumbents lack; 1k-photo pilot pricing with content-hash caching and cheap-first filtering; rescope as on-demand batch job — or deploy Immich and give the slot away.

### [UB-C9] webhook-fanin-hub: a platform for an audience of one
- **Severity**: major · **Lenses**: skeptic, user-advocate, estimator, ecosystem
- **Summary**: Meta-infrastructure with no job-to-be-done of its own: free incumbents (n8n, Pipedream, Val.town, Zapier tiers) cover personal routing; every other candidate ships without it; the gap between a two-weekend version and a pleasant one is the slate's largest (routing config, per-destination auth, retries, replay, debugging grow open-endedly). Its only honest wedge is batched fan-in semantics, which nothing yet needs.
- **Would resolve**: Defer until two shipped projects have a named route needing batched fan-in that off-the-shelf routers cannot serve; if kept, write the "why not n8n" paragraph and enforce hard v1 scope (static config, ~5 sources/destinations, no UI, no DSL, no replay).

### [UB-C10] toastmasters-ops: other people's dependency turns hobby into obligation
- **Severity**: major · **Lenses**: skeptic, user-advocate, ecosystem, operator
- **Summary**: The only candidate whose success depends on third parties: competes with entrenched free club tooling (easy-Speak, FreeToastHost, Form+Sheet); members must change behavior; once the club relies on agendas/reminders, every outage is a personal obligation with hard weekly deadlines and social cost — the most chore-prone candidate, orphaned the moment he changes role or club.
- **Would resolve**: Keep only if the club demonstrably lacks/dislikes incumbents AND it is scoped to artifacts he alone produces and consumes in an officer role he currently holds (paste RSVPs in → agenda + reminder drafts out), zero member-facing adoption; otherwise cull or park.

### [UB-C11] new-music-watcher: incumbents cover mainstream, differentiated sources lack APIs
- **Severity**: major · **Lenses**: skeptic, user-advocate, ecosystem, estimator
- **Summary**: Squeezed from both sides: Spotify Release Radar, Beatport follows, MusicHarbor already push weekly release digests for zero effort, while differentiated DJ crate-digging sources have no accessible API (Bandcamp closed years ago, Beatport partner-gated, Spotify removing hobby-dev access since late 2024), pushing toward scraping. Only worth opening if tied to his actual library and play history.
- **Would resolve**: List specific artists/labels/feeds current tools miss; verify each has a sanctioned API/RSS (MusicBrainz, label RSS, Spotify dev-mode for his own account); drop sources lacking one; make ranking library-aware via dj-library-intel data; cull if the uncovered list is small or the linkage is dropped.

### [UB-C12] llm-micro-batcher: open relay for paid keys without a declared trust boundary
- **Severity**: **blocker** · **Lenses**: security, compliance, operator
- **Summary**: As stated, an internet-facing proxy holding the builder's upstream LLM keys with no per-caller auth or spend boundary — an open relay where anyone finding the URL gets key-equivalent spending power. Centralizes the fleet's LLM traffic into a single point of failure; the sell path adds GDPR processor obligations, cross-customer prompt commingling in shared batches, custody of others' keys, de facto 24/7 on-call.
- **Would resolve**: Decide tenancy/exposure posture before building (personal-only is the sane default): per-caller auth, hard spend/rate caps with auto-shutoff, keys never logged, per-client direct-to-provider fallback; any sell path needs per-tenant isolation, retention terms, DPA — or ship self-hosted instead of operated.

### [UB-C13] No viable runtime plan: always-on windows vs free scale-to-zero tiers
- **Severity**: major · **Lenses**: operator, cost, estimator
- **Summary**: Time-window batching needs a long-lived always-on process, which assumed free hosting doesn't provide: Cloud Run request-based billing throttles CPU between requests so timers don't fire; scale-to-zero kills in-flight windows; Discord/Slack gateway bots need a persistent socket; Vercel functions are request-scoped. ~10 warm services cost roughly $50–150+/mo vs near $0 consolidated. This architecture decision changes which candidates are viable and must precede the cull.
- **Would resolve**: A one-page fleet hosting plan before culling: consolidate pipelines into one fleet-host binary on a single always-warm runtime (always-free e2-micro, or Cloud Run with CPU always allocated + min-instances=1) with a written monthly cap (≤$20/mo); each keeper names its hosting mode and how window semantics survive it.

### [UB-C14] Ten unattended services is a quota that becomes a maintenance fleet
- **Severity**: major · **Lenses**: skeptic, user-advocate, estimator
- **Summary**: Nothing requires the final number to be 10; padding keeps the weakest candidates while each deployed service costs 1–4 hrs/month to keep alive — 10–40 hrs/month of pure upkeep for an evenings-and-weekends builder whose goal is joy. Solo portfolios historically stall at ~3–5 concurrently maintained live services; fleet maintenance is the most predictable way the plan fails.
- **Would resolve**: Drop the fixed quota or tier the final 10: cap always-on services at ~4–5, run the rest as on-demand CLIs or scheduled jobs; each finalist declares run mode and a maintenance budget that sums to something a hobbyist will pay; staggered waves with a month-3 retention test.

### [UB-C15] Uncapped paid-API spend in unattended pipelines (worst: comic-render-queue)
- **Severity**: major · **Lenses**: operator, cost, security
- **Summary**: Nightly retries against paid image APIs while the operator sleeps — a poison prompt or partial outage plus a retry loop burns money unattended; likely the fleet's dominant line item ($10–60+/mo, unbounded without caps). The same exposure applies to every externally triggerable or retry-looped LLM service (llm-micro-batcher, email-triage, mood-journal, error-digester): cost exhaustion is the realistic DoS against a hobby fleet; batching smooths rates but caps nothing.
- **Would resolve**: Hard per-service spend caps and billing budgets with auto-shutoff; bounded retries with backoff and a dead-letter path; rate limits on external triggers; separate keys/budgets per project; for comic-render-queue a worked monthly budget, max-images-per-night ceiling, and an explicit nightly-vs-on-demand decision.

### [UB-C16] comic-render-queue: "retries" assume queue semantics gobatch has never had
- **Severity**: major · **Lenses**: historian, decomposer, estimator
- **Summary**: Promised retries and crash recovery require durable job state gobatch does not provide: errored items exit via the errs channel and are dropped; no retry primitive has ever existed; a Batch is single-use with no re-injection API; feeding failures back into a bounded source channel is a documented deadlock hazard. The durable job store and retry topology are the candidate's actual core.
- **Would resolve**: A one-page design before lock-in: durable job store (Firestore job docs with status/attempt), retry as in-processor bounded retry or cron rescan re-streaming failures through a fresh Batch, idempotency across restarts, gobatch demoted to rate-limit-friendly dispatch chunking.

### [UB-C17] dj-library-intel: reverse-engineered Serato formats are a fragile foundation
- **Severity**: major · **Lenses**: ecosystem, historian, estimator
- **Summary**: Depends entirely on Serato's proprietary, undocumented binary formats, readable only via reverse-engineered community parsers (none mature in Go) that lag format changes and can silently break on any Serato update; the files live on the DJ laptop, so an unscoped laptop-to-cloud ingestion step exists. Effort concentrates in format archaeology, the classic estimate-blower.
- **Would resolve**: A one-evening spike running an existing OSS parser (serato-tools, triseratops) against his real `_Serato_` files with golden-test fixtures, a pinned supported Serato version, a CSV-export fallback, and a decided laptop-to-cloud mechanism — before keeping the candidate.

### [UB-C18] error-digester: the fleet's watcher has no watcher and an LLM on the critical path
- **Severity**: major · **Lenses**: operator, user-advocate, historian
- **Summary**: The fleet's de facto monitoring system, yet nothing monitors it; built on the same gobatch engine as the services it watches (common-mode failure); an expired key, revoked webhook, or LLM quota hit silently blinds the operator to every project's failures at once; daily batching means up to 24h of blindness; LLM paraphrases can blur the actual error.
- **Would resolve**: External third-party dead-man's switch outside the gobatch path; degraded mode delivering raw error counts/samples when LLM fails (LLM as enhancement, never dependency); severity bypass notifying immediately for urgent cases; raw evidence one click from every summary.

### [UB-C19] photo-organizer: unattended destructive dedupe on irreplaceable photos
- **Severity**: major · **Lenses**: operator, user-advocate
- **Summary**: The slate's only feature that can destroy irreplaceable personal data: a hashing/heuristic bug silently deletes originals, discovered months later — past any trash/backup window; one wrongly deleted photo permanently destroys trust. Unsafe unattended as designed.
- **Would resolve**: Non-destructive contract: dedupe moves to quarantine with long retention and a reviewable report; hard-delete never automated; dry-run default; verified restore test before the first real pass.

### [UB-C20] Nine candidates share one hidden chassis; consolidate before culling
- **Severity**: major · **Lenses**: skeptic, decomposer
- **Summary**: At least nine candidates share the identical skeleton — event ingestion, gobatch micro-batch, bulk store write, scheduled digest, LLM summarize, Slack/Discord delivery — with gobatch supplying only the thin middle sliver. Built independently, the glue gets written ~9 times; mood-journal, workout-logger, discord-meme-digest, and email-triage are one product wearing four costumes; a single multi-source digest service with pluggable adapters is less ops and a stronger gobatch demonstration.
- **Would resolve**: Name the shared chassis explicitly as project #0 (ingest endpoint, batched store writer, cron digest runner, LLM summarizer, notifier — and decide whether webhook-fanin-hub IS that chassis); consolidate digest-shaped candidates; keep candidates standalone only with a genuinely different data shape or output; re-score before culling.

### [UB-C21] email-triage: duplicates Gmail's own triage and adds a second inbox
- **Severity**: major · **Lenses**: skeptic, user-advocate
- **Summary**: Re-implements what Gmail ships free (Priority Inbox, filters, tabs, native Gemini triage); as a read-only daily digest it adds a second thing to read without removing inbox work — he will keep checking Gmail anyway because triage tools have a one-strike trust bar. Downside (misclassified important mail from an unattended hobby daemon) is asymmetric to the marginal upside.
- **Would resolve**: Define the action that removes inbox work (auto-label/archive only low-stakes classes with an undo trail); two-week read-only pilot on a low-stakes subset measuring real benefit over native filters; otherwise cull.

### [UB-C22] dj-library-intel: local after-gig data forced into a cloud daemon shape
- **Severity**: major · **Lenses**: skeptic, user-advocate
- **Summary**: The data is local Serato files changing only after gigs (a few times/month), so a cloud-resident streaming service has nothing to ingest most of the time and isn't present at the laptop where set prep actually happens; a retrospective cloud dashboard becomes a vanity artifact viewed twice while forcing file-sync plumbing for zero value.
- **Would resolve**: Reframe as a local CLI or scheduled local job (gobatch batching the parse/aggregation fan-out), anchored to a named prep-time decision (e.g. "tracks in this key/BPM range unplayed in 90 days"), with at most a static published dashboard.

### [UB-C23] error-digester: rebuilds what Sentry/GCP Error Reporting do free
- **Severity**: major · **Lenses**: skeptic, ecosystem
- **Summary**: The hard parts (collection, grouping, dedupe, alert routing) are exactly what GCP Error Reporting does automatically for his stack at zero cost and what Sentry's free tier ships with Slack alerts and AI summaries; the only novel wedge is the LLM daily-digest voice plus gobatch dogfooding — a thin layer over those tools' APIs, not a standalone bespoke ingestion service.
- **Would resolve**: Wire one project into Sentry/GCP Error Reporting for a week; design error-digester to ride on existing collection (Cloud Logging sinks or Sentry webhooks feeding the gobatch digest pipeline), keeping only the digesting layer custom.

### [UB-C24] error-digester: fleet-wide secrets and PII funneled into one store and an LLM
- **Severity**: major · **Lenses**: security, compliance
- **Summary**: Centralizes error payloads from every toy; stack traces and error bodies routinely embed tokens, connection strings, request content, and third parties' data; aggregating them creates a concentrated breach target, and shipping them to an external LLM becomes an unreviewed secret-exfiltration and secondary-use channel violating each sibling project's data posture.
- **Would resolve**: Redact/structure at each source (IDs not content); filter known PII/secret fields before any LLM call; drop raw payloads after summarization; no-training/limited-retention API terms; treat the store as a high-value asset.

### [UB-C25] Platform pieces have a hidden build order that reshapes the slate
- **Severity**: major · **Lenses**: decomposer, estimator
- **Summary**: llm-micro-batcher, webhook-fanin-hub, and error-digester are infrastructure the other candidates consume or duplicate: error-digester has zero value until several services emit errors in a shared format (an integration tail touching every project, naturally hooked on the errs channel every gobatch pipeline returns); llm-micro-batcher is pure retrofit rework if siblings call providers directly first. Built late they are redundant; built early they reshape every other decomposition.
- **Would resolve**: Explicit dependency graph and declared build order; standalone-vs-chassis-module decision for each platform piece before the cull; one minimal shared error-reporting shim defined day one and adopted by every project as it ships; error-digester sequenced late.

### [UB-C26] A dozen unmanaged credentials across the fleet: leaks cascade, expiry decays
- **Severity**: major · **Lenses**: security, operator
- **Summary**: ~A dozen long-lived credential surfaces (Discord/Slack tokens, Gmail OAuth, LLM and image-gen keys, GCP service accounts) across fast-shipped, likely-public repos and free-tier hosts: a committed or host-compromised credential is likely and cascades if secrets are shared; credential expiry/revocation is the dominant slow-decay failure of personal fleets — each instance presenting as a service silently stopping plus "which key died" archaeology.
- **Would resolve**: One fleet-wide secret pattern (GCP Secret Manager runtime injection, never in-repo); pre-commit/CI secret scanning on all repos; strictly per-service credentials; a credential inventory with expiry/renewal notes.

## CONSIDER (single-lens, not promoted)

- **[UB-C27] Silent absence: dead digest pipelines look like "nothing happened"** (operator, major) — Digest-only outputs mean a dead pipeline is indistinguishable from a quiet week; breakage discovered weeks later as missing reports. *Resolve*: mandatory liveness pattern — dead-man's-switch ping per flush (healthchecks.io), zero-items alarm, shared last-success dashboard, per-source last-seen heartbeats.
- **[UB-C28] gobatch is not daemon-ready: unmerged hardening, no long-running pattern** (historian, major) — Master lacks processor panic recovery, leaks timers, can block on undrained error sends (fixes sit on unmerged branches); every example/test is run-to-completion single-use, so the always-on regime 13/14 candidates need has never been exercised. *Resolve*: land fix/engine-hardening + cancel-mode decision, cut a tagged release as the fleet floor, add a canonical long-running daemon example.
- **[UB-C29] webhook-fanin-hub: forged-webhook injection and SSRF** (security, major) — Unsigned webhooks let anyone forge events; data-driven routing enables SSRF incl. GCP metadata token theft. *Resolve*: per-source HMAC verification, static egress allowlist, block link-local/metadata/private ranges.
- **[UB-C30] webhook-fanin-hub: fire-and-forget intake means unrecoverable event loss** (operator, major) — Senders won't replay on your schedule; downtime or in-memory loss drops events permanently; the hub becomes a correlated failure domain. *Resolve*: ack-after-persist intake (durable write before 200), replay over the persisted log, dependency map.
- **[UB-C31] email-triage: Gmail token is a master key, LLM a sink for 2FA codes** (security, major) — Mailbox = recovery hub for every account; leaked broad-scope token enables takeover cascades; raw bodies to an LLM exfiltrate reset links, 2FA codes, financial data. *Resolve*: narrowest scope, redact before LLM, short-lived tokens in a secret manager, pre-written revocation plan.
- **[UB-C32] Shared GCP project/service account makes the weakest toy the fleet's front door** (security, major) — One compromised service reaches every project's data and billable compute. *Resolve*: per-project service accounts, least-privilege IAM, per-project budgets/alerts.
- **[UB-C33] llm-micro-batcher needs request-reply batching gobatch explicitly lacks** (historian, major) — Sync-like batching is README-listed unimplemented roadmap; the pipeline is one-way with no response routing, so correlation/delivery/timeouts would be hand-rolled outside the library. *Resolve*: implement sync-like batching in gobatch first, or re-scope to one-way workloads (bulk embedding backfill) where the pipeline genuinely fits.
- **[UB-C34] mood-journal: the real job is the capture habit, not the summary** (user-advocate, major) — A passive listener provides no habit scaffold so entries stop within weeks; chat platforms + third-party LLMs invite self-censorship that defeats the wellness purpose. *Resolve*: scheduled gentle streak-free prompt, stated privacy posture (private channel, redaction/local-first option), success = still capturing in week 6.
- **[UB-C35] comic-render-queue: the queue is the product, auto-comics are decaying novelty** (user-advocate, major) — Unattended nightly auto-comics are fun for days then muted slop; the joy is prompting and curation. *Resolve*: ship the queue as the product (batched submissions, overnight backlog runs); curated auto mode (weekly theme he seeds, picks from variants); kill-switch if outputs go unopened two weeks.
- **[UB-C36] Selling mood-journal or workout-logger enters the consumer-health-data regime** (compliance, major) — Personal mode is unregulated self-tracking; offering to others flips mood/fitness entries into consumer health data (WA MHMD private right of action, GDPR Art. 9, FTC HBNR), plus disclosed processors and no-training terms. Mood lines in an employer Slack are admin-exportable. *Resolve*: mark both personal-only on a personally controlled instance; any sell path is a separate gated product decision.
- **[UB-C37] Sensitive data flows to LLM/vision providers without a chosen retention posture** (security, minor) — Mood signals, private photos + GPS EXIF, email bodies to third parties whose retention/training terms he doesn't control. *Resolve*: no-retention/no-train providers (or self-host), strip GPS EXIF, document what leaves each system.
- **[UB-C38] Apply an own-data-first criterion to the cull** (ecosystem, minor) — Platform risk cleanly partitions the slate: own-data candidates are insulated; every fragile candidate (new-music-watcher, photo-organizer, gear-price-watcher, email-triage, game-session-stats) depends on a third party that closed or gated access during 2024–2026. *Resolve*: make own-data-first an explicit cull criterion; kept third-party candidates must name a sanctioned API and fallback.
- **[UB-C39] Pin a cheap model class and token budget per LLM-digest project** (cost, minor) — Digest candidates are each under ~$5/mo only with a small-model class and token budget pinned; frontier-model habits make email-triage 10–50x pricier for zero joy. *Resolve*: default Haiku/Flash-tier per project, written monthly budget, input truncation rules.
- **[UB-C40] Decide the multi-stage type-envelope pattern once, not ten ways** (decomposer, minor) — Processor chains are strictly T→T; stage-wise type changes need a fat envelope struct or chained Batches via processor.Channel→source.Channel; per-project conventions duplicate design effort and block extracting the chassis. *Resolve*: pick the pattern once in a shared internal library.
- **[UB-C41] discord-meme-digest: friends' messages to an LLM need disclosure and consent** (compliance, minor) — Harvesting others' messages/reactions and shipping identifiable content to an LLM; Discord dev terms condition message-content use on disclosure; beyond his own server triggers verified-intent territory. *Resolve*: pinned in-server disclosure with assent, strip usernames/IDs before LLM, no-training terms, own-servers-only scope.
- **[UB-C42] toastmasters-ops: member PII handling must be designed in** (compliance, minor) — Baseline mode already processes others' PII and messages them; GDPR household exemption wouldn't cover a club tool; quiet LLM piping exceeds member expectations. *Resolve*: one-time club announcement/officer sign-off, store name/contact/role-RSVP only, opt-out + delete path, template-generated agendas (no member PII to LLMs).
- **[UB-C43] photo-organizer: keep face recognition and GPS EXIF out of any shared path** (compliance, minor) — Face-template clustering becomes regulated biometric data (BIPA, CUBI, GDPR Art. 9) the moment the tool is distributed; GPS EXIF in shared output is a self-privacy footgun. *Resolve*: faces personal-only (or perceptual hashes), strip GPS from shared output, rule the face variant out of any sell path.
- **[UB-C44] discord-meme-digest: delight depends on server reaction volume** (user-advocate, minor) — A quiet server yields an empty/repetitive chaos report; "zero memes this week" is worse than silence. *Resolve*: check a month of real reaction volume first; skip-below-threshold or widen the window on sparse weeks.
- **[UB-C45] workout-logger: digest should answer the GVT progression question** (user-advocate, minor) — The one real question is "did I earn the next weight increase and what do I lift next session"; that decision-per-session keeps a tracker in use past week two. *Resolve*: digest (and on-demand bot reply) emits a next-session prescription per lift plus 2.4 km pace trend.
- **[UB-C46] comic-render-queue: image-gen API churn silently breaks nightly runs** (ecosystem, minor) — The fastest-churning API surface on the slate; a retired model ID means weeks of missing comics nobody notices at 3 a.m. *Resolve*: provider-abstraction seam, deprecation alerting, pinned model versions, quarterly sunset check.

## Coverage & violations

- **Lenses run (10)**: skeptic, operator, user-advocate, historian, decomposer, estimator, cost, ecosystem, compliance, security
- **Lenses failed**: none
- **Repo-aware**: yes (historian, decomposer on `/Users/vaughn/dev/gobatch`)
- **Violations**: none (all 10 sha256 checksums verified; no blind-lens finding cited repo evidence)
- 112 raw findings → 46 clusters → 26 MUST-RESOLVE (≥2 independent lenses) + 20 CONSIDER

## Hardened Idea — the final 10

Four candidates culled (gear-price-watcher, email-triage, game-session-stats, photo-organizer — exactly the four failing the own-data-first criterion [UB-C38]); two merged (webhook-fanin-hub → chassis ingest; new-music-watcher → dj-library-intel module); two slots refilled (gobatch hardening release; the chassis itself). Every keeper states its run mode and its honest batching rationale per [UB-C4].

**Fleet rules (apply to all):** pin to tagged gobatch releases via one shared adapter module [UB-C6]; durable ingest before batching, windows ≤ minutes, Cloud Scheduler owns cadence, SIGTERM drain [UB-C7]; one always-warm host ≤ $20/mo, everything else scheduled/on-demand/local [UB-C13, UB-C14]; per-service keys + billing budgets + auto-shutoff + bounded retries + dead-letter [UB-C15]; Secret Manager injection + secret scanning + per-service credentials + per-project service accounts [UB-C26, UB-C32]; dead-man's-switch liveness on every flush [UB-C27]; Haiku/Flash-tier defaults with written token budgets [UB-C39]; shared envelope pattern [UB-C40]; no-train/no-retention LLM terms for personal content [UB-C37].

1. **gobatch v0.x "daemon-grade" release** — *the floor.* Land the engine-hardening branches (panic recovery, timer leak, blocked error sends), decide cancel semantics, add the canonical long-running daemon example (never-closing source, supervised re-New, graceful drain) and an idle-soak loss test to CI, tag and pin. Run mode: repo work. Batching rationale: it IS the batching. [UB-C6, C7, C28]
2. **Blobworks (the chassis)** — *one fleet host, one shared skeleton.* Single Go binary on an always-free e2-micro (or Cloud Run min-instances=1): ack-after-persist ingest endpoint (HMAC-verified, static config, ≤5 sources, egress allowlist, no UI/DSL), batched Firestore bulk writer, cron digest runner, LLM summarizer with raw-mode fallback, Slack/Discord notifier, liveness pings, error-shim client. Absorbs webhook-fanin-hub as its ingest front door. Run mode: the one always-on resident. Batching: bulk-write coalescing + notifier rate-limit chunking — the fleet's genuine stream. [UB-C9, C20, C29, C30, C25]
3. **Vibe Ledger (mood-journal)** — chassis adapter. One-liner mood drops in a private personally-controlled Discord channel; scheduled gentle streak-free capture prompt; redaction before LLM; weekly vibe report. Success metric: still capturing in week 6. Personal-only (health-data regime if ever sold). Run mode: module on Blobworks. Batching: digest-side re-stream of stored rows, LLM context chunking. [UB-C34, C36, C7]
4. **Volume Goblin (workout-logger)** — chassis adapter. Quick-message sets/runs → durable write → weekly digest that answers the GVT question: next-session prescription per lift + 2.4 km pace trend; on-demand bot reply. Personal-only. Run mode: module on Blobworks. Batching: bulk Firestore writes + digest fan-out. [UB-C45, C36]
5. **Chaos Report (discord-meme-digest)** — chassis adapter. Harvest reactions on his own server (pinned disclosure, usernames stripped before LLM, no-training terms); weekly ranked chaos digest; skip-below-threshold on quiet weeks; volume pre-check for a month before building. Run mode: module on Blobworks (gateway socket lives in the host). Batching: reaction-event micro-batches → bulk store; digest chunking. [UB-C41, C44]
6. **Crate Sage (dj-library-intel + Fresh Drops module)** — *local-first.* Scheduled job on the DJ laptop parsing `_Serato_` files (gated on a one-evening OSS-parser spike with golden fixtures, pinned Serato version, CSV-export fallback); answers named prep-time questions ("tracks in this key/BPM range unplayed 90 days; gaps worth buying"); publishes a static dashboard. Fresh Drops: library-aware new-release digest from sanctioned feeds only (MusicBrainz, label/artist RSS, Spotify dev-mode on his own account) — any source without an API/feed is dropped. Run mode: local scheduled + static publish. Batching: count-window fan-out over thousands of tracks/files, bulk aggregation. [UB-C17, C22, C11]
7. **Panel Foundry (comic-render-queue)** — the queue is the product. Durable Firestore job docs (status/attempt), cron rescan re-streams failures through a fresh Batch, idempotent dispatch, bounded retries + dead-letter; curated auto mode (he seeds a weekly theme, picks from variants), kill-switch if unopened 2 weeks; worked monthly budget + max-images-per-night ceiling; provider seam with pinned model versions + quarterly sunset check. Run mode: Cloud Scheduler job. Batching: rate-limit-friendly dispatch chunking against image APIs — a genuinely honest fit. [UB-C16, C15, C35, C46]
8. **Vectorsmith (ex llm-micro-batcher)** — *rescoped one-way.* Bulk embedding backfill/enrichment worker: chunks thousands of texts (journal entries, comics metadata, library notes) into array-sized embedding calls with rate pacing, feeding personal semantic search across his own stuff. Personal-only, never internet-exposed, no sell path; sync request-reply batching stays a gobatch roadmap item, not a dependency. Run mode: on-demand CLI/job. Batching: array-fill count windows — the most honest micro-batching on the slate. [UB-C5, C12, C33]
9. **Agenda Golem (toastmasters-ops)** — *officer-artifact mode.* Paste RSVPs/signups in → agenda + reminder drafts out; he alone produces and consumes the artifacts; zero member-facing adoption required; template-generated (member PII never sent to LLMs); one-time club sign-off; store name/contact/role only with delete path. Cull trigger: role lapses. Run mode: on-demand/weekly scheduled job. Batching: RSVP-row fan-out into one agenda build. [UB-C10, C42]
10. **Daily Doom (error-digester)** — *thin layer, built last.* GCP Error Reporting/Sentry free tier owns collection/grouping; Daily Doom reads their APIs and renders the deadpan daily chaos digest via the chassis; external dead-man's switch (healthchecks.io) outside the gobatch path; raw-counts degraded mode when LLM fails; severity bypass for first-error-from-silent-service; redaction at source via the day-one error shim (IDs not content). Run mode: daily scheduled job, sequenced wave 4. Batching: error-event grouping + digest chunking. [UB-C18, C23, C24, C25]

**Build order (waves, month-3 retention test between):** W1: gobatch release → Blobworks + error shim → Vibe Ledger. W2: Volume Goblin, Chaos Report, Panel Foundry. W3: Crate Sage, Vectorsmith, Agenda Golem. W4: Daily Doom.

**Sell verdict:** run nothing as a paid operated service. Mood/fitness data sold = consumer-health-data regime [UB-C36]; proxy-for-others = processor obligations + on-call [UB-C12]. The realistic commercial assets: gobatch itself (reputation/OSS), Panel Foundry and Blobworks as self-hosted OSS templates with a sponsor button. Revisit selling only after an external user names a concrete gap in writing.

<!-- ULTRA-BRAINSTORM:BEGIN run=20260612T061517Z-gobatch-10-projects -->
### Ultra-Brainstorm decisions (2026-06-12)
- [UB-C1] gear-price-watcher: solved by incumbents, rest is a scraping treadmill — **RESOLVED** — Culled; incumbents cover it, scraping is ToS-breaching with silent-staleness failure; own-data-first criterion adopted.
- [UB-C2] email-triage: Gmail restricted-scope OAuth breaks "unattended" by design — **RESOLVED** — Culled; no survivable unattended auth for a solo consumer-Gmail app (also C21/C31).
- [UB-C3] game-session-stats: the data source does not exist — **RESOLVED** — Culled; no safe capture path exists (anti-cheat risk on his real account), manual entry makes play a chore.
- [UB-C4] Slate-wide: micro-batching is decorative at personal event volumes — **RESOLVED** — Constraint reframed: every keeper records expected volume + concrete batching benefit + where batching occurs; cron-shaped keepers are scheduled jobs whose internal fan-out uses gobatch.
- [UB-C5] llm-micro-batcher: weak personal-scale premise, "sellable" unrealistic — **RESOLVED** — Rescoped to Vectorsmith: one-way bulk embedding backfill (array-fill batching, real consumer = personal semantic search); sellable framing dropped.
- [UB-C6] Fleet on v0 gobatch: every breaking change multiplies by ten — **RESOLVED** — Tag releases; pin every project to a tag; one shared adapter module absorbs breakage; quarterly fleet upgrade pass; no candidate depends on future gobatch features.
- [UB-C7] In-memory batch windows silently lose data; persist-before-batch needed — **RESOLVED** — Fleet reference architecture: durable ingest before batching, windows ≤ minutes, Scheduler owns cadence, SIGTERM drain; idle-soak loss test added to gobatch release scope.
- [UB-C8] photo-organizer: source closed, problem already solved, costs unpriced — **RESOLVED** — Culled; deploy Immich instead; slot reassigned to the chassis.
- [UB-C9] webhook-fanin-hub: a platform for an audience of one — **RESOLVED** — Culled as standalone; reborn as Blobworks ingest front door (static config, ≤5 sources, HMAC, egress allowlist, ack-after-persist, no UI/DSL/replay v1).
- [UB-C10] toastmasters-ops: other people's dependency turns hobby into obligation — **RESOLVED** — Rescoped to officer-artifact mode (paste RSVPs in → drafts out, zero member adoption); documented cull trigger when role lapses.
- [UB-C11] new-music-watcher: incumbents cover mainstream, differentiated sources lack APIs — **RESOLVED** — Merged into Crate Sage as the Fresh Drops module: library-aware ranking, sanctioned APIs/RSS only, drop-source rule.
- [UB-C12] llm-micro-batcher: open relay for paid keys without a declared trust boundary — **RESOLVED** — Vectorsmith is personal-only and never internet-exposed (CLI/job, no endpoint); any future sell path pre-gated on per-tenant isolation/DPA or self-hosted distribution.
- [UB-C13] No viable runtime plan: always-on windows vs free scale-to-zero tiers — **RESOLVED** — One always-warm fleet host (e2-micro or Cloud Run min-instances=1, CPU always allocated) runs Blobworks + adapters, cap ≤ $20/mo; everything else scheduled/on-demand/local; each keeper declares run mode.
- [UB-C14] Ten unattended services is a quota that becomes a maintenance fleet — **RESOLVED** — Tiered: 10 projects but exactly 1 always-on host (3 adapters + ingest in it); rest scheduled jobs, on-demand CLIs, local jobs, or repo work; staged waves with month-3 retention test.
- [UB-C15] Uncapped paid-API spend in unattended pipelines — **RESOLVED** — Fleet rule: per-service keys + billing budgets + auto-shutoff, bounded retries + dead-letter, rate-limited triggers; Panel Foundry gets worked budget + nightly image ceiling + curated mode.
- [UB-C16] comic-render-queue: "retries" assume queue semantics gobatch has never had — **RESOLVED** — Durable Firestore job docs are the queue; cron rescan re-streams failures through a fresh Batch; idempotency keys; gobatch demoted to dispatch chunking.
- [UB-C17] dj-library-intel: reverse-engineered Serato formats are a fragile foundation — **RESOLVED** — Keep gated on a one-evening OSS-parser spike vs his real _Serato_ files + golden fixtures + pinned version + CSV fallback; local-first removes the laptop-to-cloud sync entirely.
- [UB-C18] error-digester: the fleet's watcher has no watcher, LLM on critical path — **RESOLVED** — External dead-man's switch outside the gobatch path; raw-counts degraded mode; severity bypass; raw evidence linked; collection layer is external (GCP/Sentry) so fleet data survives digester outages.
- [UB-C19] photo-organizer: unattended destructive dedupe on irreplaceable photos — **RESOLVED** — Moot via cull; non-destructive contract (quarantine, dry-run default, no automated hard-delete) adopted as a fleet rule for anything touching user files.
- [UB-C20] Nine candidates share one hidden chassis; consolidate before culling — **RESOLVED** — Chassis named explicitly (Blobworks, slate slot #2); webhook-fanin-hub IS the chassis ingest; digest products become thin adapters; slate re-scored on that basis.
- [UB-C21] email-triage: duplicates Gmail's own triage and adds a second inbox — **RESOLVED** — Moot via cull; native Gmail triage + asymmetric trust downside judged decisive.
- [UB-C22] dj-library-intel: local after-gig data forced into a cloud daemon shape — **RESOLVED** — Reframed local-first: scheduled laptop job + static published dashboard, anchored to named prep-time decisions.
- [UB-C23] error-digester: rebuilds what Sentry/GCP Error Reporting do free — **RESOLVED** — Daily Doom rides existing collection (Error Reporting/Sentry APIs); only the digest layer is custom.
- [UB-C24] error-digester: fleet-wide secrets and PII funneled into one store and an LLM — **RESOLVED** — Redaction at source via the day-one error shim (IDs not content), PII/secret filtering before LLM, redacted-summaries-only retention, no-training terms.
- [UB-C25] Platform pieces have a hidden build order that reshapes the slate — **RESOLVED** — Explicit wave plan (gobatch → chassis+shim → adapters → Daily Doom last); standalone-vs-module decided for all three platform pieces; error shim adopted day one.
- [UB-C26] A dozen unmanaged credentials across the fleet — **RESOLVED** — Secret Manager runtime injection (never in-repo), secret scanning in the project template, strictly per-service credentials, credential inventory with expiry notes.
<!-- ULTRA-BRAINSTORM:END -->

## Audit trail

All 26 MUST-RESOLVE clusters were walked in rank order on 2026-06-12 (run executed autonomously; each decision is recorded above keyed by `UB-<cluster_id>` and can be individually overturned — a re-run updates lines in place). Resolution timestamps: 2026-06-12T06:42:30Z.

| Cluster | State | Decision (short) |
|---|---|---|
| UB-C1 | RESOLVED | Cull gear-price-watcher |
| UB-C2 | RESOLVED | Cull email-triage |
| UB-C3 | RESOLVED | Cull game-session-stats |
| UB-C4 | RESOLVED | Honest-batching rule per keeper; cron-shaped → scheduled jobs |
| UB-C5 | RESOLVED | Rescope to Vectorsmith (one-way embedding backfill) |
| UB-C6 | RESOLVED | Tagged releases + pins + shared adapter + quarterly upgrade pass |
| UB-C7 | RESOLVED | Durable-ingest-first reference architecture; idle-soak CI test |
| UB-C8 | RESOLVED | Cull photo-organizer; use Immich |
| UB-C9 | RESOLVED | Merge webhook-fanin-hub into Blobworks ingest |
| UB-C10 | RESOLVED | Rescope toastmasters-ops to officer-artifact mode |
| UB-C11 | RESOLVED | Merge new-music-watcher into Crate Sage (Fresh Drops) |
| UB-C12 | RESOLVED | Vectorsmith personal-only, never exposed; sell path pre-gated |
| UB-C13 | RESOLVED | One always-warm fleet host, ≤ $20/mo cap |
| UB-C14 | RESOLVED | Tiered run modes: 1 always-on host; waves + retention test |
| UB-C15 | RESOLVED | Spend caps/budgets/auto-shutoff fleet rule; Panel Foundry ceiling |
| UB-C16 | RESOLVED | Firestore job docs + cron-rescan retry design |
| UB-C17 | RESOLVED | Parser spike gate + fixtures + CSV fallback; local-first |
| UB-C18 | RESOLVED | External dead-man switch; degraded raw mode; severity bypass |
| UB-C19 | RESOLVED | Moot (cull); non-destructive contract as fleet rule |
| UB-C20 | RESOLVED | Chassis named (Blobworks); digest products = adapters |
| UB-C21 | RESOLVED | Moot (cull email-triage) |
| UB-C22 | RESOLVED | Crate Sage local-first reframe |
| UB-C23 | RESOLVED | Daily Doom rides GCP Error Reporting/Sentry |
| UB-C24 | RESOLVED | Redact at source; filter before LLM; summaries-only retention |
| UB-C25 | RESOLVED | Wave build order; error shim day one; Daily Doom last |
| UB-C26 | RESOLVED | Secret Manager + scanning + per-service creds + inventory |

**Verdict: 26 resolved, 0 waived, 0 deferred.**
