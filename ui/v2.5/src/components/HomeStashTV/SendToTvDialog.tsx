import React, { useEffect, useMemo, useState } from "react";
import { Alert, Form } from "react-bootstrap";
import { Link } from "react-router-dom";
import { ModalComponent } from "src/components/Shared/Modal";
import { useToast } from "src/hooks/Toast";
import {
  BridgeClient,
  BridgeError,
  IReceiver,
  homeStashTvTargetKey,
  loadHomeStashTvSettings,
  saveHomeStashTvSettings,
  selectInitialHomeStashTvTarget,
} from "src/models/homeStashTv/BridgeClient";

interface ISendToTvDialog {
  sceneIDs?: readonly string[];
  resolveSceneIDs?: () => Promise<string[]>;
  startPositionMs?: number;
  onClose: () => void;
}

function receiverStatus(receiver: IReceiver) {
  if (receiver.revoked) return "revoked";
  if (
    receiver.protocol_version !== undefined &&
    receiver.protocol_version !== null &&
    receiver.protocol_version !== 1
  ) {
    return "incompatible";
  }
  return receiver.online ? "online" : "offline";
}

function receiverOptions(receivers: IReceiver[]) {
  return receivers.flatMap((receiver) => {
    const status = receiverStatus(receiver);
    return receiver.profiles.map((profile) => ({
      receiverId: receiver.receiver_id,
      profileId: profile.id,
      deviceName: receiver.device_name,
      profileName: profile.name,
      online: receiver.online,
      disabled: receiver.revoked || status === "incompatible",
      status,
    }));
  });
}

export const SendToTvDialog: React.FC<ISendToTvDialog> = ({
  sceneIDs,
  resolveSceneIDs,
  startPositionMs = 0,
  onClose,
}) => {
  const Toast = useToast();
  const settings = useMemo(() => loadHomeStashTvSettings(), []);
  const [receivers, setReceivers] = useState<IReceiver[]>([]);
  const [selectedTarget, setSelectedTarget] = useState("");
  const [loading, setLoading] = useState(!!settings);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string>();

  const targets = useMemo(() => receiverOptions(receivers), [receivers]);
  const availableTargets = useMemo(
    () => targets.filter((target) => !target.disabled),
    [targets]
  );

  useEffect(() => {
    if (!settings) return;

    const client = new BridgeClient(settings);
    client
      .listReceivers()
      .then((value) => {
        setReceivers(value);
        const options = receiverOptions(value).filter(
          (target) => !target.disabled
        );
        setSelectedTarget(
          selectInitialHomeStashTvTarget(options, settings.preferredTarget)
        );
      })
      .catch((cause) => {
        setError(
          cause instanceof Error ? cause.message : "Could not load TV devices."
        );
      })
      .finally(() => setLoading(false));
  }, [settings]);

  async function onSend() {
    if (!settings) return;

    const target = availableTargets.find(
      (candidate) => homeStashTvTargetKey(candidate) === selectedTarget
    );
    if (!target) {
      setError("Choose an available TV and Stash profile.");
      return;
    }

    setSending(true);
    setError(undefined);
    try {
      const resolvedSceneIDs = sceneIDs
        ? [...sceneIDs]
        : await resolveSceneIDs?.();
      if (!resolvedSceneIDs?.length) {
        throw new Error("The TV queue contains no scenes.");
      }

      const updatedSettings = saveHomeStashTvSettings({
        bridgeUrl: settings.bridgeUrl,
        senderToken: settings.senderToken,
        preferredTarget: {
          receiverId: target.receiverId,
          profileId: target.profileId,
        },
      });
      const client = new BridgeClient(updatedSettings);
      const submission = await client.sendQueue({
        receiver_id: target.receiverId,
        profile_id: target.profileId,
        scene_ids: resolvedSceneIDs,
        start_index: 0,
        start_position_ms: Math.max(0, Math.floor(startPositionMs)),
        policy: {
          continue: true,
          loop: true,
          reshuffle: resolvedSceneIDs.length > 1,
        },
      });
      const acknowledgement = await client.waitForAcknowledgement(
        submission.command_id
      );

      if (
        acknowledgement.status === "expired" ||
        acknowledgement.status === "rejected"
      ) {
        throw new BridgeError(
          acknowledgement.errorCode
            ? `TV rejected the queue: ${acknowledgement.errorCode}`
            : `TV delivery ${acknowledgement.status}.`,
          acknowledgement.status
        );
      }

      const message =
        acknowledgement.status === "queued"
          ? `${target.deviceName} is offline. The queue is saved until it reconnects.`
          : acknowledgement.status === "duplicate"
          ? `${target.deviceName} already received this queue.`
          : `Queue delivered to ${target.deviceName}.`;
      Toast.success(message);
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Send to TV failed.");
    } finally {
      setSending(false);
    }
  }

  const selected = availableTargets.find(
    (target) => homeStashTvTargetKey(target) === selectedTarget
  );

  return (
    <ModalComponent
      show
      header="Send to TV"
      onHide={onClose}
      cancel={{ onClick: onClose }}
      accept={{ text: "Send", onClick: onSend }}
      isRunning={sending}
      disabled={loading || !settings || !selected || selected.disabled}
    >
      {!settings ? (
        <Alert variant="warning">
          Configure the native bridge in{" "}
          <Link to="/settings?tab=home-stash-tv" onClick={onClose}>
            Settings → Home Stash TV
          </Link>
          .
        </Alert>
      ) : (
        <>
          {error && <Alert variant="danger">{error}</Alert>}
          {!loading && receivers.length === 0 && (
            <Alert variant="warning">
              No paired TVs were found. Start pairing in Home Stash TV and
              approve it on the bridge host first.
            </Alert>
          )}
          {!loading && receivers.length > 0 && targets.length === 0 && (
            <Alert variant="warning">
              Paired TVs have not advertised any Stash profiles.
            </Alert>
          )}
          <Form.Group>
            <Form.Label>TV and Stash profile</Form.Label>
            <Form.Control
              as="select"
              value={selectedTarget}
              disabled={loading || sending}
              onChange={(event) => {
                setSelectedTarget(event.currentTarget.value);
                setError(undefined);
              }}
            >
              {targets.length > 0 && !selectedTarget && (
                <option value="">Choose a TV and Stash profile</option>
              )}
              {targets.length === 0 && (
                <option value="">No target found</option>
              )}
              {targets.map((target) => (
                <option
                  key={homeStashTvTargetKey(target)}
                  value={homeStashTvTargetKey(target)}
                  disabled={target.disabled}
                >
                  {target.deviceName} — {target.profileName} ({target.status})
                </option>
              ))}
            </Form.Control>
            {!loading && availableTargets.length > 1 && !selected && (
              <Form.Text className="text-warning">
                Multiple TV and Stash profile targets are available. Choose the
                intended target explicitly; Home Stash will not guess.
              </Form.Text>
            )}
            {selected && !selected.online && (
              <Form.Text className="text-warning">
                This TV is offline. The bridge will wake it and retain the
                command until it reconnects or expires.
              </Form.Text>
            )}
          </Form.Group>
        </>
      )}
    </ModalComponent>
  );
};

export default SendToTvDialog;
