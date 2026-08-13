const SETTINGS_STORAGE_KEY = "homeStash.tv.settings.v1";
const PROTOCOL_VERSION = 1;

export interface IHomeStashTvTarget {
  receiverId: string;
  profileId: string;
}

export function homeStashTvTargetKey(target: IHomeStashTvTarget) {
  return `${target.receiverId}:${target.profileId}`;
}

export function selectInitialHomeStashTvTarget(
  targets: readonly IHomeStashTvTarget[],
  preferredTarget?: IHomeStashTvTarget
) {
  const preferred = preferredTarget
    ? homeStashTvTargetKey(preferredTarget)
    : undefined;
  if (
    preferred &&
    targets.some((target) => homeStashTvTargetKey(target) === preferred)
  ) {
    return preferred;
  }
  return targets.length === 1 ? homeStashTvTargetKey(targets[0]) : "";
}

export interface IHomeStashTvSettings {
  version: 1;
  bridgeUrl: string;
  senderToken: string;
  preferredTarget?: IHomeStashTvTarget;
}

export interface IReceiverProfile {
  id: string;
  name: string;
}

export interface IPlaybackState {
  command_id: string;
  state:
    | "resolving"
    | "playing"
    | "paused"
    | "stopped"
    | "completed"
    | "failed";
  scene_id?: string | null;
  queue_index?: number | null;
  position_ms?: number | null;
  error_code?: string | null;
  skipped_scene_ids: string[];
  updated_at_ms: number;
}

export interface IReceiver {
  receiver_id: string;
  device_name: string;
  revoked: boolean;
  created_at_ms: number;
  profiles: IReceiverProfile[];
  playback_state?: IPlaybackState | null;
  online: boolean;
  protocol_version?: number | null;
  app_version?: string | null;
  last_seen_ms?: number | null;
}

export interface IQueuePolicy {
  continue: boolean;
  loop: boolean;
  reshuffle: boolean;
}

export interface ISendQueueInput {
  receiver_id: string;
  profile_id: string;
  scene_ids: string[];
  start_index?: number;
  start_position_ms?: number;
  policy: IQueuePolicy;
}

export interface ICommandSubmission {
  v: number;
  status: "pending";
  wake_status: "requested" | "failed";
  command_id: string;
  receiver_id: string;
  expires_at_ms: number;
  receiver_online?: boolean;
}

export interface ICommandStatus {
  v: number;
  command_id: string;
  receiver_id: string;
  state: "pending" | "acknowledged" | "expired";
  ack_status?: "accepted" | "duplicate" | "expired" | "rejected" | null;
  ack_error_code?: string | null;
  created_at_ms: number;
  expires_at_ms: number;
}

export type CommandAcknowledgement =
  | { status: "accepted" | "duplicate"; command: ICommandStatus }
  | { status: "queued"; command: ICommandStatus }
  | {
      status: "expired" | "rejected";
      command: ICommandStatus;
      errorCode?: string | null;
    };

type Fetch = typeof fetch;

export class BridgeError extends Error {
  public readonly code: string;
  public readonly status?: number;

  constructor(message: string, code: string, status?: number) {
    super(message);
    this.name = "BridgeError";
    this.code = code;
    this.status = status;
  }
}

function browserStorage() {
  return typeof window === "undefined" ? undefined : window.localStorage;
}

export function normalizeBridgeUrl(value: string) {
  const trimmed = value.trim().replace(/\/+$/, "");
  if (!trimmed) {
    throw new BridgeError("Bridge URL is required.", "settings_missing");
  }

  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    throw new BridgeError("Bridge URL is invalid.", "settings_invalid");
  }

  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new BridgeError(
      "Bridge URL must use HTTP or HTTPS.",
      "settings_invalid"
    );
  }
  if (parsed.username || parsed.password || parsed.search || parsed.hash) {
    throw new BridgeError(
      "Bridge URL cannot contain credentials, a query, or a fragment.",
      "settings_invalid"
    );
  }

  return parsed.toString().replace(/\/+$/, "");
}

export function loadHomeStashTvSettings(): IHomeStashTvSettings | undefined {
  try {
    const raw = browserStorage()?.getItem(SETTINGS_STORAGE_KEY);
    if (!raw) return undefined;

    const value = JSON.parse(raw) as Partial<IHomeStashTvSettings>;
    if (
      value.version !== 1 ||
      typeof value.bridgeUrl !== "string" ||
      typeof value.senderToken !== "string" ||
      !value.bridgeUrl ||
      !value.senderToken
    ) {
      return undefined;
    }

    return {
      version: 1,
      bridgeUrl: normalizeBridgeUrl(value.bridgeUrl),
      senderToken: value.senderToken,
      preferredTarget: value.preferredTarget,
    };
  } catch {
    return undefined;
  }
}

export function saveHomeStashTvSettings(
  value: Omit<IHomeStashTvSettings, "version">
) {
  const senderToken = value.senderToken.trim();
  if (!senderToken) {
    throw new BridgeError("Sender token is required.", "settings_missing");
  }

  const settings: IHomeStashTvSettings = {
    version: 1,
    bridgeUrl: normalizeBridgeUrl(value.bridgeUrl),
    senderToken,
    preferredTarget: value.preferredTarget,
  };
  browserStorage()?.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(settings));
  return settings;
}

export function clearHomeStashTvSettings() {
  browserStorage()?.removeItem(SETTINGS_STORAGE_KEY);
}

function safeErrorMessage(
  value: unknown,
  fallback: string,
  sensitiveValue: string
) {
  if (value && typeof value === "object" && "error" in value) {
    const message = (value as { error?: unknown }).error;
    if (typeof message === "string" && message.length <= 200) {
      return message.split(sensitiveValue).join("[redacted]");
    }
  }
  return fallback;
}

export class BridgeClient {
  private readonly bridgeUrl: string;
  private readonly senderToken: string;
  private readonly fetchImpl: Fetch;

  constructor(settings: IHomeStashTvSettings, fetchImpl?: Fetch) {
    this.bridgeUrl = normalizeBridgeUrl(settings.bridgeUrl);
    this.senderToken = settings.senderToken;

    // Browser-native fetch is receiver-sensitive. Bind it before storing it on
    // the client so `this.fetchImpl(...)` cannot rebind `this` to BridgeClient.
    this.fetchImpl = fetchImpl ?? globalThis.fetch.bind(globalThis);
  }

  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    let response: Response;
    try {
      response = await this.fetchImpl(`${this.bridgeUrl}${path}`, {
        ...init,
        headers: {
          Accept: "application/json",
          Authorization: `Bearer ${this.senderToken}`,
          ...(init?.body ? { "Content-Type": "application/json" } : {}),
          ...init?.headers,
        },
      });
    } catch {
      throw new BridgeError(
        "Home Stash TV bridge is unreachable.",
        "bridge_offline"
      );
    }

    let value: unknown;
    try {
      value = await response.json();
    } catch {
      value = undefined;
    }

    if (!response.ok) {
      const code =
        response.status === 401
          ? "sender_unauthorized"
          : response.status === 410
          ? "command_expired"
          : "bridge_rejected";
      throw new BridgeError(
        safeErrorMessage(
          value,
          `Bridge request failed (${response.status}).`,
          this.senderToken
        ),
        code,
        response.status
      );
    }

    return value as T;
  }

  public async listReceivers() {
    const value = await this.request<{
      v: number;
      receivers: IReceiver[];
    }>("/api/v1/receivers");

    if (value.v !== PROTOCOL_VERSION || !Array.isArray(value.receivers)) {
      throw new BridgeError(
        "Bridge protocol is incompatible with this Home Stash build.",
        "protocol_incompatible"
      );
    }
    return value.receivers;
  }

  public async sendQueue(input: ISendQueueInput) {
    if (!input.scene_ids.length || input.scene_ids.length > 500) {
      throw new BridgeError(
        "A TV queue must contain between 1 and 500 scenes.",
        "queue_size"
      );
    }

    return this.request<ICommandSubmission>("/api/v1/commands", {
      method: "POST",
      body: JSON.stringify(input),
    });
  }

  public async commandStatus(commandId: string) {
    return this.request<ICommandStatus>(
      `/api/v1/commands/${encodeURIComponent(commandId)}`
    );
  }

  public async waitForAcknowledgement(
    commandId: string,
    timeoutMs = 10_000,
    pollMs = 350
  ): Promise<CommandAcknowledgement> {
    const deadline = Date.now() + timeoutMs;
    let current = await this.commandStatus(commandId);

    while (current.state === "pending" && Date.now() < deadline) {
      await new Promise((resolve) => setTimeout(resolve, pollMs));
      current = await this.commandStatus(commandId);
    }

    if (current.state === "pending") {
      return { status: "queued", command: current };
    }
    if (current.state === "expired" || current.ack_status === "expired") {
      return {
        status: "expired",
        command: current,
        errorCode: current.ack_error_code,
      };
    }
    if (current.ack_status === "rejected") {
      return {
        status: "rejected",
        command: current,
        errorCode: current.ack_error_code,
      };
    }
    if (
      current.ack_status === "accepted" ||
      current.ack_status === "duplicate"
    ) {
      return { status: current.ack_status, command: current };
    }

    return { status: "queued", command: current };
  }
}
