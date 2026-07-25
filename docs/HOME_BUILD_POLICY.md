# Home Stash build policy

This repository is a frozen personal fork of Stash for the Home Systems Engineering project.

## Stable branch

- `home-v0.31.1` is the stable deployment branch.
- It is based on upstream tag `v0.31.1`.
- Upstream changes are not merged automatically.

## Development flow

1. Create a temporary `feature/...` branch from `home-v0.31.1`.
2. Keep each change narrowly scoped and covered by tests where practical.
3. Build and validate a canary image before changing a live instance.
4. Merge only reviewed and tested work into `home-v0.31.1`.
5. Delete the temporary feature branch after merge.

## Images

- Never publish or deploy a mutable `latest` tag.
- Every image tag must identify the Home Stash release or source commit.
- Record the image digest and source commit in `Yahigod/home-server`.
- Keep the prior known-good image available until both Normal Stash and javStash pass validation.

## Upstream policy

Upstream Stash is a source of optional fixes and features, not an automatic update stream. Changes may be cherry-picked only after reviewing their relevance, migration impact, and compatibility with Home Stash modifications.

## Licensing

Preserve the upstream AGPL-3.0 license and keep corresponding modified source available in this repository for every distributed custom image.
