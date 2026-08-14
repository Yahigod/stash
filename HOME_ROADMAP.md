# Home Stash roadmap

Related deployment tracking: `Yahigod/home-server#50`

## Goal

Build a reproducible custom Stash version with deterministic continuous playback:

- Directly opened scenes loop indefinitely.
- One-item queues loop indefinitely.
- Multi-scene queues play every scene exactly once per cycle.
- Completed cycles reshuffle before starting again.
- The first scene of a new cycle differs from the final scene of the previous cycle.
- For three or more scenes, avoid repeating the exact previous order.
- Manual Next, Previous, Shuffle, refresh, and starting a new queue continue to work.
- The behavior works with Send to TV.

## Baseline and maintenance policy

- [x] Fork created at `Yahigod/stash`.
- [x] Frozen branch created from upstream tag `v0.31.1`: `home-v0.31.1`.
- [x] `HOME_CHANGES.md` added.
- [ ] Confirm the baseline branch commit matches the currently deployed Stash build.
- [ ] Do not merge upstream automatically.
- [ ] Use immutable custom image tags and record image digests.
- [ ] Preserve AGPL-3.0 license and corresponding source availability.

## Phase 1 — Reproducible baseline build

- [ ] Clone the fork separately from `home-server`.
- [ ] Add `upstream` remote pointing to `stashapp/stash`.
- [ ] Check out `home-v0.31.1`.
- [ ] Build an unmodified baseline image.
- [ ] Verify the unmodified image reports the expected Stash version.
- [ ] Run available upstream unit/UI tests.
- [ ] Document build prerequisites and exact commands.
- [ ] Tag the baseline image immutably.

## Phase 2 — Safe canary environment

- [ ] Record the currently deployed official image ID/digest.
- [ ] Write the rollback procedure before changing the live deployment.
- [ ] Take and verify a cold backup of Normal Stash database/config.
- [ ] Create a canary instance with copied config/database.
- [ ] Mount media read-only where practical.
- [ ] Use a separate address/port and prevent accidental scheduled jobs or destructive writes.
- [ ] Confirm the baseline custom image behaves identically to the official image.

## Phase 3 — Fixed-order looping in source

- [ ] Add source-level direct-scene looping.
- [ ] Add one-item queue looping.
- [ ] Wrap two-item queues `A → B → A`.
- [ ] Wrap three-item queues `A → B → C → A`.
- [ ] Preserve Continue behavior.
- [ ] Preserve manual Next.
- [ ] Preserve manual Previous.
- [ ] Preserve refresh and queue state.
- [ ] Starting a new queue replaces the old queue.
- [ ] Native Shuffle remains functional.
- [ ] Add automated tests for queue-boundary behavior.

## Phase 4 — Reshuffle between cycles

- [ ] Use Fisher–Yates or equivalent unbiased shuffle.
- [ ] Every scene appears exactly once per cycle.
- [ ] No duplicate or skipped scene occurs within a cycle.
- [ ] A new cycle's first scene differs from the previous final scene.
- [ ] For three or more scenes, avoid the exact previous order.
- [ ] Two-scene queues alternate without repeating the boundary scene.
- [ ] One-scene queues remain simple loops.
- [ ] Manual Shuffle establishes a new cycle order.
- [ ] Add deterministic tests with injectable randomness.

## Phase 5 — Queue coverage

- [ ] Manually selected `qs` queue.
- [ ] Filtered Scenes-page queue.
- [ ] Group/playlist queue.
- [ ] Random/shuffled queue.
- [ ] Direct scene with no queue.
- [ ] Large filtered queues avoid unreasonable client-side state.
- [ ] Large/random queues can restart with a fresh seed where appropriate.

## Phase 6 — Send to TV integration

- [ ] Define a stable interface between Stash queue logic and the existing TV bridge.
- [ ] Preserve selected scene order on the first cycle.
- [ ] Continue through every selected scene.
- [ ] Reshuffle only after the completed cycle.
- [ ] Preserve fullscreen and autoplay.
- [ ] Test when the TV is already on.
- [ ] Test when the TV is initially off.
- [ ] Preserve WoL and fixed TCP ADB reconnect behavior.
- [ ] Ensure wallpaper/random mode does not replace selected queues.

## Phase 7 — Normal Stash rollout

- [ ] Build a reviewed immutable image from a specific commit.
- [ ] Record image tag, source commit, and digest in `home-server`.
- [ ] Deploy only Normal Stash first.
- [ ] Validate scanning, metadata, plugins, browsing, playback, queue navigation, and backups.
- [ ] Validate one full cycle and one reshuffled cycle on PC.
- [ ] Validate one full cycle and one reshuffled cycle on TV.
- [ ] Run without playback/navigation regressions for an agreed observation period.

## Phase 8 — javStash rollout

- [ ] Back up javStash database/config.
- [ ] Deploy the same reviewed image.
- [ ] Validate direct looping and queue cycling.
- [ ] Validate reshuffling.
- [ ] Validate manual Next/Previous/Shuffle.
- [ ] Validate Send to TV with the television on and off.

## Rollback

- [ ] Keep the exact previous official image reference.
- [ ] Keep Compose/data mounts unchanged by image swaps.
- [ ] Document one-command rollback for Normal Stash.
- [ ] Test rollback in the canary environment.
- [ ] Do not remove old images or backups until both instances pass.

## Completion criteria

- [ ] All automated tests pass.
- [ ] Baseline and custom image builds are reproducible.
- [ ] Normal Stash passes final PC and TV validation.
- [ ] javStash passes final PC and TV validation.
- [ ] `home-server` records deployed image digests and rollback instructions.
- [ ] Work is complete only after both instances have passed final testing.

## Post-MVP follow-up — browser-independent bridge access

This follow-up does not block the Normal or javStash MVP acceptance gates above.
It is the first planned improvement before the browser integration is considered
polished. System integration, deployment, and acceptance are tracked in
[`Yahigod/home-server#294`](https://github.com/Yahigod/home-server/issues/294).

- [ ] Replace per-browser bridge URLs, sender tokens, and Local Network Access
      workarounds with a fixed same-origin server gateway.
- [ ] Keep the raw sender token in a private, read-only server secret; never
      return it to frontend JavaScript or place it in browser storage, images,
      source control, Compose, logs, or diagnostics.
- [ ] Pin one reviewed bridge destination and forward only receiver discovery,
      play-queue command creation, and command-status reads.
- [ ] Reject client-selected upstreams, arbitrary paths or methods, pairing and
      administrative routes, and all generic proxy behavior.
- [ ] Require authenticated Stash access and retain origin/CSRF checks, bounded
      requests and responses, timeouts, schema validation, and audit evidence.
- [ ] Verify the macvlan reachability boundary before implementation; do not
      change shared network or IPAM state as an incidental workaround.
- [ ] Add deterministic authentication, allowlist, SSRF, secret non-disclosure,
      timeout, and response-bound tests.
- [ ] Validate fresh Firefox and LibreWolf profiles without manual bridge setup
      or browser local-network preference changes.
- [ ] Retain the browser-direct transport until the replacement passes Normal
      and javStash end-to-end acceptance and has a documented rollback.
