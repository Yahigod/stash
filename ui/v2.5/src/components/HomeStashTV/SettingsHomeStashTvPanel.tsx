import React, { useEffect, useMemo, useState } from "react";
import { Alert, Badge, Button, Form, Table } from "react-bootstrap";
import { useToast } from "src/hooks/Toast";
import {
  BridgeClient,
  IHomeStashTvSettings,
  IReceiver,
  clearHomeStashTvSettings,
  loadHomeStashTvSettings,
  saveHomeStashTvSettings,
} from "src/models/homeStashTv/BridgeClient";

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
  const [bridgeUrl, setBridgeUrl] = useState(saved?.bridgeUrl ?? "");
  const [senderToken, setSenderToken] = useState(saved?.senderToken ?? "");
  const [receivers, setReceivers] = useState<IReceiver[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  async function refresh(settings?: IHomeStashTvSettings) {
    const value = settings ?? loadHomeStashTvSettings();
    if (!value) return;

    setLoading(true);
    setError(undefined);
    try {
      setReceivers(await new BridgeClient(value).listReceivers());
    } catch (cause) {
      setReceivers([]);
      setError(
        cause instanceof Error ? cause.message : "Could not reach the bridge."
      );
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (saved) refresh(saved);
  }, [saved]);

  async function saveAndTest() {
    setLoading(true);
    setError(undefined);
    try {
      const settings = saveHomeStashTvSettings({
        bridgeUrl,
        senderToken,
        preferredTarget: saved?.preferredTarget,
      });
      setBridgeUrl(settings.bridgeUrl);
      setSenderToken(settings.senderToken);
      const value = await new BridgeClient(settings).listReceivers();
      setReceivers(value);
      Toast.success("Home Stash TV bridge settings saved.");
    } catch (cause) {
      setReceivers([]);
      setError(
        cause instanceof Error ? cause.message : "Could not save TV settings."
      );
    } finally {
      setLoading(false);
    }
  }

  function clear() {
    clearHomeStashTvSettings();
    setBridgeUrl("");
    setSenderToken("");
    setReceivers([]);
    setError(undefined);
    Toast.success("Home Stash TV settings cleared from this browser.");
  }

  return (
    <div>
      <h2>Home Stash TV</h2>
      <p>
        Connect this browser to the LAN-only native receiver bridge. The bridge
        address and sender token stay in this browser and are never compiled
        into Home Stash.
      </p>

      {error && <Alert variant="danger">{error}</Alert>}

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
        <Form.Text className="text-muted">
          This is the bridge sender token, not a Stash API key.
        </Form.Text>
      </Form.Group>
      <div className="mb-4">
        <Button
          className="mr-2"
          disabled={loading || !bridgeUrl.trim() || !senderToken.trim()}
          onClick={saveAndTest}
        >
          {loading ? "Testing…" : "Save and test"}
        </Button>
        <Button variant="secondary" disabled={loading} onClick={clear}>
          Clear
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
        Pairing and revocation remain bridge-host actions. This page discovers
        paired TVs and lets each Home Stash origin remember its preferred
        device/profile target.
      </Alert>
    </div>
  );
};

export default SettingsHomeStashTvPanel;
