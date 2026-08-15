# Home Stash TV same-origin gateway

The Home Stash TV gateway lets the built-in Stash UI submit bounded playback
commands without giving frontend JavaScript the bridge destination or sender
token. The browser calls its own Stash origin. Stash then calls one fixed,
operator-reviewed bridge origin.

This is not a generic reverse proxy. It mounts only:

- `GET /api/home-stash-tv/v1/receivers`
- `POST /api/home-stash-tv/v1/commands`
- `GET /api/home-stash-tv/v1/commands/{command-id}`

Pairing, approval, revocation, receiver WebSockets, arbitrary paths, and other
methods are not exposed.

## Preconditions

- Configure Stash login credentials. The gateway accepts an authenticated
  browser session only; a Stash API key is not accepted.
- Prove the Stash container can reach the reviewed bridge origin without an
  incidental shared-network or IPAM change.
- Create a distinct, revocable sender credential where the bridge credential
  model supports it.
- Keep the browser-direct transport available until gateway acceptance is
  complete.

## Process configuration

Set both variables before Stash starts:

```text
STASH_TV_GATEWAY_URL=http://reviewed-bridge-host:8791
STASH_TV_GATEWAY_TOKEN_FILE=/run/secrets/home-stash-tv-sender-token
```

The URL must be exactly one HTTP(S) origin. It cannot contain credentials, a
path, a query, or a fragment.

The token path must be absolute and canonical. It must point directly to a
private regular file, not a symlink, and contain only the sender token with an
optional final newline. Mount the file read-only and keep the actual token out
of Compose, environment values, images, source control, command lines, and
logs.

If neither variable is set, the gateway route remains unavailable and a
browser that already has legacy direct settings may use them. If only one
variable is set or either value fails validation, Stash startup fails closed.

## Browser migration

1. Start Stash with the reviewed gateway configuration.
2. Sign in from a fresh browser profile.
3. Open **Settings → Home Stash TV** and confirm **Server gateway connected**.
4. Run receiver discovery and Send to TV acceptance without entering a bridge
   URL/token or changing browser Local Network Access preferences.
5. Repeat for every supported Stash origin and the required browser matrix.
6. Only after acceptance, use **Clear old browser-held bridge settings** in
   previously configured profiles.

The client falls back only when the gateway is absent or unavailable. A failed
Stash login, origin check, or CSRF check does not downgrade to direct transport.

## Token rotation

1. Create a new sender credential on the bridge host without revoking the old
   credential yet.
2. Write it to a new private regular secret file using the deployment secret
   mechanism. Never edit the mounted file in place.
3. Restart one Stash instance with the new secret-file identity and verify
   receiver discovery plus one bounded command.
4. Roll the second Stash instance only after the first passes.
5. Revoke the old bridge credential.
6. Confirm the old credential no longer authorizes discovery or commands and
   retain only non-secret rotation evidence.

Do not print either credential or copy it into an issue, diagnostic, Compose
render, shell history, or CI output.

## Rollback

Rollback is a configuration-only operation:

1. Restore the exact prior Stash process configuration and image.
2. Restart only the affected Stash service.
3. Confirm the gateway route is unavailable and ordinary Stash health is
   restored.
4. Use a browser profile with retained legacy direct settings for temporary
   Send to TV access.

Do not revoke the legacy sender credential or clear browser settings until the
replacement has passed end-to-end acceptance. Rollback does not require a
database restore or a bridge administrative change.

## Acceptance and non-disclosure evidence

For both Normal and JAV Stash, verify:

- an authenticated fresh Firefox, LibreWolf, and Chromium-family profile can
  discover the production receiver and send a queue;
- no profile needs a bridge URL, sender token, CORS exception, or Local Network
  Access exception;
- unauthenticated, cross-origin, missing-CSRF, API-key, unsupported-method, and
  unsupported-path requests fail;
- bounded timeout, oversized-body, malformed-schema, redirect, and
  client-supplied-upstream tests fail closed;
- the token is absent from HTML, JavaScript bundles, browser storage, API
  responses, ordinary logs, diagnostics, Compose, image history, and Git
  history.

Audit records may contain only the gateway route class, method, status, and
duration. They must not contain command, receiver, profile, scene, URL, or
credential values.
