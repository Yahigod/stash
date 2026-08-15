# Home Stash changes

This fork is the source of truth for George's self-hosted Stash builds.

## Frozen baseline

- Upstream repository: `stashapp/stash`
- Baseline release: `v0.31.1`
- Long-lived branch: `home-v0.31.1`
- Update policy: do not merge upstream automatically; review upstream only when it offers a wanted feature, relevant fix, security repair, or compatibility improvement.

## Custom changes

- Directly opened scenes and one-item queues loop continuously.
- Explicit and filtered queues continue through every item and reshuffle only
  after a completed cycle.
- Multi-scene cycles avoid repeating their boundary scene and, for three or
  more scenes, avoid repeating the complete prior order.
- Manual Next, Previous, Shuffle, refresh, and replacement-queue behavior stay
  available.
- Send to TV preserves the same reviewed queue policy through the native Home
  Stash TV receiver.
- A narrow same-origin Home Stash TV gateway keeps the fixed bridge destination
  and sender credential on the server. It exposes only receiver discovery,
  command creation, and command-status reads to authenticated browser sessions.
- The browser-direct bridge transport remains temporarily available as a
  bounded migration and rollback path.

Deployment, rotation, migration, and rollback are documented in
[`HOME_STASH_TV_GATEWAY.md`](HOME_STASH_TV_GATEWAY.md).

## Build and release policy

- Never publish or deploy a mutable `latest` tag.
- Build immutable versioned images from reviewed commits.
- Record the source commit and image digest for every deployment.
- Keep the existing Stash data/config mounts independent of the container image.
- Maintain and test an explicit rollback to the previous official image.

## Compatibility

Stash is licensed under AGPL-3.0. Preserve upstream license notices and make the corresponding modified source available with distributed builds.

Re-test all custom playback behavior before adopting any future upstream release.
