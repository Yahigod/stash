const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const ts = require("typescript");

function loadTypeScript(relativePath) {
  const filename = path.join(__dirname, relativePath);
  const source = fs.readFileSync(filename, "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
    },
    fileName: filename,
  }).outputText;
  const loadedModule = { exports: {} };
  const load = new Function(
    "exports",
    "module",
    "require",
    "__filename",
    "__dirname",
    transpiled
  );
  load(
    loadedModule.exports,
    loadedModule,
    require,
    filename,
    path.dirname(filename)
  );
  return loadedModule.exports;
}

const {
  BridgeClient,
  BridgeError,
  clearHomeStashTvSettings,
  loadHomeStashTvSettings,
  normalizeBridgeUrl,
  saveHomeStashTvSettings,
} = loadTypeScript("../src/models/homeStashTv/BridgeClient.ts");
const { resolveFilteredSceneIDs } = loadTypeScript(
  "../src/models/homeStashTv/queue.ts"
);

function response(status, value) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function settings() {
  return {
    version: 1,
    bridgeUrl: "http://bridge.test:8791",
    senderToken: "sender-token-with-at-least-thirty-two-characters",
  };
}

test("bridge URL validation accepts HTTP origins and rejects embedded credentials", () => {
  assert.equal(
    normalizeBridgeUrl(" http://bridge.test:8791/// "),
    "http://bridge.test:8791"
  );
  assert.throws(
    () => normalizeBridgeUrl("http://user:secret@bridge.test"),
    BridgeError
  );
  assert.throws(() => normalizeBridgeUrl("file:///tmp/bridge"), BridgeError);
});

test("settings stay browser-local and can be cleared", () => {
  const values = new Map();
  const originalWindow = global.window;
  global.window = {
    localStorage: {
      getItem: (key) => values.get(key) ?? null,
      setItem: (key, value) => values.set(key, value),
      removeItem: (key) => values.delete(key),
    },
  };

  try {
    const saved = saveHomeStashTvSettings({
      bridgeUrl: "http://bridge.test:8791/",
      senderToken: " secret-token ",
      preferredTarget: { receiverId: "receiver", profileId: "profile" },
    });
    assert.equal(saved.senderToken, "secret-token");
    assert.deepEqual(loadHomeStashTvSettings(), saved);
    clearHomeStashTvSettings();
    assert.equal(loadHomeStashTvSettings(), undefined);
  } finally {
    global.window = originalWindow;
  }
});

test("fake bridge discovery uses sender authentication and preserves device states", async () => {
  const requests = [];
  const fakeFetch = async (url, init) => {
    requests.push({ url, init });
    return response(200, {
      v: 1,
      receivers: [
        {
          receiver_id: "living-room",
          device_name: "Living Room TV",
          revoked: false,
          created_at_ms: 1,
          profiles: [{ id: "normal", name: "Normal Stash" }],
          online: true,
          protocol_version: 1,
          app_version: "1.0",
        },
        {
          receiver_id: "old-tv",
          device_name: "Old TV",
          revoked: false,
          created_at_ms: 1,
          profiles: [],
          online: false,
          protocol_version: 0,
        },
      ],
    });
  };

  const receivers = await new BridgeClient(
    settings(),
    fakeFetch
  ).listReceivers();
  assert.equal(receivers[0].online, true);
  assert.equal(receivers[1].protocol_version, 0);
  assert.equal(requests[0].url, "http://bridge.test:8791/api/v1/receivers");
  assert.equal(
    requests[0].init.headers.Authorization,
    `Bearer ${settings().senderToken}`
  );
});

test("fake bridge command keeps selected order, start position, and policy", async () => {
  let request;
  const fakeFetch = async (url, init) => {
    request = { url, init };
    return response(202, {
      v: 1,
      status: "pending",
      wake_status: "requested",
      command_id: "command-1",
      receiver_id: "living-room",
      expires_at_ms: 123,
    });
  };
  const client = new BridgeClient(settings(), fakeFetch);

  await client.sendQueue({
    receiver_id: "living-room",
    profile_id: "normal",
    scene_ids: ["9", "2", "7"],
    start_index: 1,
    start_position_ms: 5000,
    policy: { continue: true, loop: true, reshuffle: true },
  });

  assert.equal(request.url, "http://bridge.test:8791/api/v1/commands");
  assert.deepEqual(JSON.parse(request.init.body), {
    receiver_id: "living-room",
    profile_id: "normal",
    scene_ids: ["9", "2", "7"],
    start_index: 1,
    start_position_ms: 5000,
    policy: { continue: true, loop: true, reshuffle: true },
  });
});

test("delivery polling distinguishes acknowledgement, rejection, expiry, and queueing", async () => {
  for (const [state, ackStatus, expected] of [
    ["acknowledged", "accepted", "accepted"],
    ["acknowledged", "duplicate", "duplicate"],
    ["acknowledged", "rejected", "rejected"],
    ["expired", null, "expired"],
  ]) {
    const fakeFetch = async () =>
      response(200, {
        v: 1,
        command_id: "command",
        receiver_id: "receiver",
        state,
        ack_status: ackStatus,
        ack_error_code: ackStatus === "rejected" ? "profile_missing" : null,
        created_at_ms: 1,
        expires_at_ms: 2,
      });
    const result = await new BridgeClient(
      settings(),
      fakeFetch
    ).waitForAcknowledgement("command", 0, 0);
    assert.equal(result.status, expected);
  }

  const pendingFetch = async () =>
    response(200, {
      v: 1,
      command_id: "command",
      receiver_id: "receiver",
      state: "pending",
      created_at_ms: 1,
      expires_at_ms: 2,
    });
  const queued = await new BridgeClient(
    settings(),
    pendingFetch
  ).waitForAcknowledgement("command", 0, 0);
  assert.equal(queued.status, "queued");
});

test("bridge errors never include the configured sender token", async () => {
  const fakeFetch = async () =>
    response(401, { error: `bad token ${settings().senderToken}` });
  const client = new BridgeClient(settings(), fakeFetch);
  await assert.rejects(client.listReceivers(), (error) => {
    assert.equal(error.code, "sender_unauthorized");
    assert.equal(error.message.includes(settings().senderToken), false);
    return true;
  });
});

test("filtered queue resolution paginates without changing order", async () => {
  class Filter {
    currentPage = 7;
    itemsPerPage = 40;
    clone() {
      return Object.assign(new Filter(), this);
    }
  }
  const pages = [
    Array.from({ length: 100 }, (_, index) => ({ id: String(index + 1) })),
    Array.from({ length: 2 }, (_, index) => ({ id: String(index + 101) })),
  ];
  const seenPages = [];
  const result = await resolveFilteredSceneIDs(new Filter(), async (filter) => {
    seenPages.push(filter.currentPage);
    return {
      data: {
        findScenes: {
          count: 102,
          scenes: pages[filter.currentPage - 1],
        },
      },
    };
  });

  assert.deepEqual(seenPages, [1, 2]);
  assert.equal(result.length, 102);
  assert.deepEqual(result.slice(-3), ["100", "101", "102"]);
});

test("filtered queues larger than the bridge limit fail before submission", async () => {
  const filter = {
    currentPage: 1,
    itemsPerPage: 40,
    clone() {
      return { ...this };
    },
  };
  await assert.rejects(
    resolveFilteredSceneIDs(filter, async () => ({
      data: { findScenes: { count: 501, scenes: [{ id: "1" }] } },
    })),
    /500 or fewer/
  );
});
