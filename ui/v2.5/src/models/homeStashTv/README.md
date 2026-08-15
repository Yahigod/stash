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

## Same-origin gateway is the primary transport

The primary transport is the server-side route rooted at
`/api/home-stash-tv`. The browser sends only same-origin requests and retains
only its preferred receiver/profile target. It never receives or stores the
bridge destination or sender token.

The server gateway is deliberately narrower than the bridge API:

- receiver discovery, command creation, and command-status reads are allowed;
- pairing, approval, revocation, receiver WebSockets, arbitrary paths, and
  arbitrary methods are not mounted;
- the upstream is one operator-configured HTTP(S) origin and cannot be chosen
  by a browser request;
- request and response bodies, JSON schemas, redirects, and request duration
  are bounded;
- a Stash login session, a same-origin browser signal, and
  `X-Stash-TV-CSRF: 1` are all required;
- Stash API keys do not authorize this browser-only gateway.

Enable the gateway only when Stash credentials are configured. Set both
`STASH_TV_GATEWAY_URL` and `STASH_TV_GATEWAY_TOKEN_FILE` before process start.
The token file path must be absolute and canonical, point directly to a private
regular file, and contain only the sender token. Partial or unsafe
configuration stops Stash rather than silently weakening the boundary.

The gateway logs only its bounded route name, method, status, and duration. Do
not add receiver IDs, scene IDs, command bodies, upstream response bodies, or
the token to logs.

## Legacy direct browser transport is a rollback path

The prior browser-direct transport remains available during migration and
rollback. It requires all of these independent browser security layers:

1. The Stash page Content Security Policy permits the configured HTTP or HTTPS
   bridge connection.
2. The bridge allows the exact Stash browser origin through CORS.
3. CORS preflight allows the request method and the `Authorization`, `Accept`,
   and `Content-Type` headers used by the client.
4. Browsers that enforce Local Network Access permit the page origin to contact
   LAN resources.

The UI tries this path only when the same-origin gateway is absent or
unavailable and legacy settings already exist in that browser. Authentication,
origin, and CSRF failures never downgrade to the legacy transport. After the
gateway passes end-to-end acceptance, clear the old browser-held settings.

## Acceptance must exercise the real UI

Checkpoint acceptance must originate from the built-in Home Stash interface:

- sign in to Stash and confirm the settings panel reports that the server
  gateway is connected without entering a bridge URL or token;
- discover the paired receiver and its profiles;
- submit current-scene, ordered-selection, and filtered-queue commands through
  the visible Send to TV actions;
- observe the resulting command state and native receiver behavior.

Repeat this from a fresh Firefox, LibreWolf, and Chromium-family browser
profile. No browser should need a Local Network Access exception. Confirm that
HTML, built assets, browser storage, API responses, and ordinary logs contain
neither the sender token nor the fixed upstream.

Direct bridge API calls are useful for diagnosis and observation, but they are
not a substitute for UI acceptance.

## Fresh worktrees require GraphQL generation

`src/core/generated-graphql.ts` is generated rather than tracked. In a new
checkout or worktree, run:

    npm run gqlgen

before running `npm run check` or a frontend build. Without the generated file,
TypeScript reports a large cascade of missing-module and secondary type errors
that are unrelated to the current change.
