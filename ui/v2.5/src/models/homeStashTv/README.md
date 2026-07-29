# Home Stash TV frontend development notes

These notes record non-obvious implementation and acceptance constraints for
the browser side of Home Stash TV.

## Native browser functions must retain their receiver

Some browser-native functions are receiver-sensitive. Do not store an unbound
native function such as `fetch` on a class and later invoke it as an object
method:

    this.fetchImpl = fetch;
    await this.fetchImpl(url);

The method call can rebind `this` to the class instance. Chromium may then throw
before creating a network request.

Bind the native function to the runtime global before storing it:

    this.fetchImpl = globalThis.fetch.bind(globalThis);

Injected test functions may remain unbound. Ordinary JavaScript fake functions
often tolerate arbitrary receivers, so a regression test must explicitly use a
receiver-sensitive replacement when testing this behavior.

A typical symptom is that a direct page-context `fetch()` succeeds while the
typed client reports that the bridge is unreachable and browser network
instrumentation sees no request.

## LAN browser transport has separate security layers

A working Home Stash TV browser connection requires all of these:

1. The Stash page Content Security Policy must permit the configured HTTP or
   HTTPS bridge connection.
2. The bridge must allow the exact Stash browser origin through CORS.
3. CORS preflight must allow the request method and the `Authorization`,
   `Accept`, and `Content-Type` headers used by the client.
4. Browsers that enforce Local Network Access must permit the page origin to
   contact LAN resources.

Test these layers separately. A successful command-line request does not prove
that a browser request is permitted.

## Acceptance must exercise the real UI

Checkpoint acceptance must originate from the built-in Home Stash interface:

- configure the bridge through the Home Stash TV settings panel;
- discover the paired receiver and its profiles;
- submit current-scene, ordered-selection, and filtered-queue commands through
  the visible Send to TV actions;
- observe the resulting command state and native receiver behavior.

Direct bridge API calls are useful for diagnosis and observation, but they are
not a substitute for UI acceptance.

## Fresh worktrees require GraphQL generation

`src/core/generated-graphql.ts` is generated rather than tracked. In a new
checkout or worktree, run:

    npm run gqlgen

before running `npm run check` or a frontend build. Without the generated file,
TypeScript reports a large cascade of missing-module and secondary type errors
that are unrelated to the current change.
