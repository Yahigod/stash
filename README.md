# Home Stash

Custom Stash build maintained for the **Home Systems Engineering** project.

This repository is the source of truth for the Stash version deployed on the home server. It is intentionally frozen on a known-good baseline and changed only when a feature, fix, or operational need is worth carrying permanently.

## Purpose

Home Stash exists to provide a stable, reproducible media-library application with behavior tailored to this environment.

The first major customization is continuous playback:

- directly opened scenes loop indefinitely;
- queues play every scene exactly once per cycle;
- completed cycles reshuffle before restarting;
- cycle boundaries avoid immediately repeating the same scene;
- manual Next, Previous, Shuffle, refresh, and queue replacement continue to work;
- Send to TV uses the same queue rules.

Future changes will be added only when they provide clear value to the home setup.

## Baseline

- **Frozen application version:** `v0.31.1`
- **Stable branch:** `home-v0.31.1`
- **Deployment model:** immutable custom Docker images
- **Update policy:** no automatic upstream synchronization

The stable branch is treated like appliance firmware. Changes are developed on temporary feature branches, tested, reviewed, and merged only after rollback is prepared.

## Repository documents

- [`HOME_CHANGES.md`](HOME_CHANGES.md) — every permanent difference carried by this build
- [`HOME_ROADMAP.md`](HOME_ROADMAP.md) — full implementation and validation checklist
- [`docs/HOME_BUILD_POLICY.md`](docs/HOME_BUILD_POLICY.md) — branching, building, testing, image tagging, and rollback policy
- [Issue #1](../../issues/1) — source-level tracking for continuous playback and TV integration

Deployment configuration, backups, image digests, and rollback commands are tracked in the `home-server` repository.

## Branch model

- `home-v0.31.1` — stable, deployable source
- `feature/<name>` — temporary development work

Feature branches are removed after merge. The stable branch is never force-pushed during normal development.

## Versioning

Source releases and Docker images use matching immutable identifiers.

Example:

```text
Source tag:   home-0.31.1-1
Docker image: ghcr.io/yahigod/stash:home-0.31.1-1
Source commit: recorded in deployment documentation
Image digest: recorded in deployment documentation
```

The `v0.31.1` tag marks the untouched baseline. Custom `home-*` tags will mark reviewed Home Stash builds.

## Deployment order

1. Build and verify an unchanged baseline image.
2. Test changes in an isolated canary environment.
3. Deploy to Normal Stash first.
4. Validate browser playback, queues, plugins, metadata, scanning, backups, and Send to TV.
5. Observe for regressions.
6. Deploy to javStash only after Normal Stash passes.

The previous image and verified backups are retained until both instances pass final validation.

## Maintenance policy

Home Stash does not follow upstream releases automatically. A newer version is considered only when it contains something this installation actually needs, such as:

- a relevant security fix;
- a data-integrity or database fix;
- required browser, codec, or platform compatibility;
- a feature worth adopting;
- a fix for a problem present in this deployment.

Any adopted change must be reviewed and tested against the current Home Stash behavior before deployment.

## License

The existing project license remains in [`LICENSE`](LICENSE). Modified source and build history remain available in this repository.
