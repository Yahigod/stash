# Home Stash changes

This fork is the source of truth for George's self-hosted Stash builds.

## Frozen baseline

- Upstream repository: `stashapp/stash`
- Baseline release: `v0.31.1`
- Long-lived branch: `home-v0.31.1`
- Update policy: do not merge upstream automatically; review upstream only when it offers a wanted feature, relevant fix, security repair, or compatibility improvement.

## Custom changes

No source changes yet.

Planned work:

- Loop directly opened scenes indefinitely.
- Continue explicit and filtered queues through every item.
- Reshuffle completed multi-scene cycles.
- Prevent the first scene of a new cycle from matching the final scene of the previous cycle.
- Preserve manual Next, Previous, Shuffle, refresh, and new-queue behavior.
- Integrate the behavior with Send to TV.
- Validate Normal Stash before javStash.

## Build and release policy

- Never publish or deploy a mutable `latest` tag.
- Build immutable versioned images from reviewed commits.
- Record the source commit and image digest for every deployment.
- Keep the existing Stash data/config mounts independent of the container image.
- Maintain and test an explicit rollback to the previous official image.

## Compatibility

Stash is licensed under AGPL-3.0. Preserve upstream license notices and make the corresponding modified source available with distributed builds.

Re-test all custom playback behavior before adopting any future upstream release.
