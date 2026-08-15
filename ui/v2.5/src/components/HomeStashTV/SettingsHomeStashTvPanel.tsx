import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Badge, Button, Form, Table } from "react-bootstrap";
import { useToast } from "src/hooks/Toast";
import {
  BridgeClient,
  IHomeStashTvSettings,
  IReceiver,
  clearHomeStashTvSettings,
  loadHomeStashTvSettings,
  saveHomeStashTvSettings,
  shouldTryLegacyHomeStashTvTransport,
} from "src/models/homeStashTv/BridgeClient";

type TransportState = "checking" | "gateway" | "direct" | "unavailable";

function status(receiver: IReceiver) {
  if (receiver.revoked) return { label: "Revoked", variant: "danger" };
  if (
    receiver.protocol_version !== undefined &&
    receiver.protocol_version !== null &&
    receiver.protocol_version !== 1
  ) {
    return { label: "Incompatible", variant: "danger" };
  }
  if (receiver.online) return { label: "Online", variant: "success" };
  return { label: "Offline", variant: "secondary" };
}

export const SettingsHomeStashTvPanel: React.FC = () => {
  const Toast = useToast();
  const saved = useMemo(() => loadHomeStashTvSettings(), []);
  const [legacySettings, setLegacySettings] = useState<
    IHomeStashTvSettings | undefined
  >(saved);
  const [bridgeUrl, setBridgeUrl] = useState(saved?.bridgeUrl ?? "");
  const [senderToken, setSenderToken] = useState(saved?.senderToken ?? "");
  const [receivers, setReceivers] = useState<IReceiver[]>([]);
  const [transport, setTransport] = useState<TransportState>("checking");
  const [legacyFallbackAllowed, setLegacyFallbackAllowed] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  const refresh = useCallback(
    async (preferredClient?: BridgeClient) => {
      setLoading(true);
      setError(undefined);
      try {
        if (preferredClient) {
          setReceivers(await preferredClient.listReceivers());
          setTransport(preferredClient.transport);
          setLegacyFallbackAllowed(preferredClient.transport === "direct");
          return;
        }

        const gateway = new BridgeClient();
        try {
          setReceivers(await gateway.listReceivers());
          setTransport("gateway");
          setLegacyFallbackAllowed(false);
          return;
        } catch (cause) {
          const fallbackAllowed = shouldTryLegacyHomeStashTvTransport(cause);
          setLegacyFallbackAllowed(fallbackAllowed);
          if (!legacySettings || !fallbackAllowed) {
            throw cause;
          }
        }

        const direct = new BridgeClient(legacySettings);
        setReceivers(await direct.listReceivers());
        setTransport("direct");
        setLegacyFallbackAllowed(true);
      } catch (cause) {
        setReceivers([]);
        setTransport("unavailable");
        setError(
          cause instanceof Error
            ? cause.message
            : "Could not reach Home Stash TV."
        );
      } finally {
        setLoading(false);
      }
    },
    [legacySettings]
  );

  useEffect(() => {
    refresh();
  }, [refresh]);

  async function saveAndTest() {
    setLoading(true);
    setError(undefined);
    try {
      const settings = saveHomeStashTvSettings({
        bridgeUrl,
        senderToken,
        preferredTarget: legacySettings?.preferredTarget,
      });
      setLegacySettings(settings);
      setBridgeUrl(settings.bridgeUrl);
      setSenderToken(settings.senderToken);
      await refresh(new BridgeClient(settings));
      Toast.success("Legacy direct bridge settings saved in this browser.");
    } catch (cause) {
      setReceivers([]);
      setTransport("unavailable");
      setError(
        cause instanceof Error ? cause.message : "Could not save TV settings."
      );
      setLoading(false);
    }
  }

  function clearLegacy() {
    clearHomeStashTvSettings();
    setLegacySettings(undefined);
    setBridgeUrl("");
    setSenderToken("");
    Toast.success("Legacy browser-held bridge settings cleared.");
    refresh(new BridgeClient());
  }

  const showLegacySetup =
    transport === "direct" ||
    (transport === "unavailable" && legacyFallbackAllowed);

  return (
    <div>
      <h2>Home Stash TV</h2>
      <p>
        Home Stash connects to the LAN-only native receiver bridge through a
        narrow server-side gateway. The fixed bridge address and sender token
        are not returned to this browser.
      </p>

      {transport === "gateway" && (
        <Alert variant="success">
          Server gateway connected. No per-browser bridge URL, sender token,
          CORS, or Local Network Access exception is required.
        </Alert>
      )}
      {transport === "direct" && (
        <Alert variant="warning">
          The server gateway is unavailable, so this browser is using the
          retained legacy direct connection.
        </Alert>
      )}
      {error && <Alert variant="danger">{error}</Alert>}

      {showLegacySetup && (
        <Alert variant="secondary">
          <Alert.Heading>Legacy direct fallback</Alert.Heading>
          <p>
            Keep this only while the server gateway is being rolled out or
            rolled back. It stores the bridge address and sender token in this
            browser.
          </p>
          <Form.Group>
            <Form.Label>Bridge URL</Form.Label>
            <Form.Control
              type="url"
              value={bridgeUrl}
              placeholder="http://bridge-host:8791"
              autoComplete="off"
              onChange={(event) => setBridgeUrl(event.currentTarget.value)}
            />
          </Form.Group>
          <Form.Group>
            <Form.Label>Sender token</Form.Label>
            <Form.Control
              type="password"
              value={senderToken}
              autoComplete="new-password"
              onChange={(event) => setSenderToken(event.currentTarget.value)}
            />
          </Form.Group>
          <Button
            className="mr-2"
            disabled={loading || !bridgeUrl.trim() || !senderToken.trim()}
            onClick={saveAndTest}
          >
            {loading ? "Testing…" : "Save and test legacy fallback"}
          </Button>
          <Button
            variant="secondary"
            disabled={loading || (!bridgeUrl && !senderToken)}
            onClick={clearLegacy}
          >
            Clear legacy settings
          </Button>
        </Alert>
      )}

      {transport === "gateway" && legacySettings && (
        <div className="mb-3">
          <Button variant="secondary" disabled={loading} onClick={clearLegacy}>
            Clear old browser-held bridge settings
          </Button>
        </div>
      )}
      <div className="mb-4">
        <Button disabled={loading} onClick={() => refresh()}>
          {loading ? "Checking…" : "Refresh TVs"}
        </Button>
      </div>

      <h3>Paired TVs</h3>
      {!loading && receivers.length === 0 ? (
        <p>No paired TVs are available.</p>
      ) : (
        <Table responsive striped>
          <thead>
            <tr>
              <th>Device</th>
              <th>Status</th>
              <th>Stash profiles</th>
              <th>App</th>
            </tr>
          </thead>
          <tbody>
            {receivers.map((receiver) => {
              const currentStatus = status(receiver);
              return (
                <tr key={receiver.receiver_id}>
                  <td>{receiver.device_name}</td>
                  <td>
                    <Badge variant={currentStatus.variant}>
                      {currentStatus.label}
                    </Badge>
                  </td>
                  <td>
                    {receiver.profiles.length
                      ? receiver.profiles
                          .map((profile) => profile.name)
                          .join(", ")
                      : "No profiles advertised"}
                  </td>
                  <td>{receiver.app_version ?? "Unknown"}</td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      )}

      <Alert variant="info">
        Pairing and revocation remain bridge-host actions. This page exposes
        only paired-TV discovery. Each browser may remember its preferred TV and
        Stash profile, but it does not need the sender token.
      </Alert>
    </div>
  );
};

export default SettingsHomeStashTvPanel;
