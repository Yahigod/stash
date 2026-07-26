const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const ts = require("typescript");

function loadQueueCycleModule() {
  const filename = path.join(__dirname, "../src/models/sceneQueueCycle.ts");
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
  buildNextSelectedQueueCycle,
  clearSelectedQueueCycle,
  fisherYatesShuffle,
  isFilteredQueueCycleCandidateValid,
  persistSelectedQueueCycle,
} = loadQueueCycleModule();

function scene(id) {
  return { id };
}

function sequenceRandom(values) {
  let index = 0;

  return () => {
    assert.ok(index < values.length, "random sequence was exhausted");
    const value = values[index];
    index += 1;
    return value;
  };
}

test("Fisher-Yates uses injectable randomness without mutating its input", () => {
  const scenes = [scene("A"), scene("B"), scene("C")];
  const shuffled = fisherYatesShuffle(scenes, sequenceRandom([0, 0]));

  assert.deepEqual(
    shuffled.map(({ id }) => id),
    ["B", "C", "A"]
  );
  assert.deepEqual(
    scenes.map(({ id }) => id),
    ["A", "B", "C"]
  );
});

test("one-scene selected queues loop unchanged", () => {
  const scenes = [scene("A")];

  assert.deepEqual(buildNextSelectedQueueCycle(scenes, "A"), scenes);
});

test("two-scene queues preserve A-B order across a completed boundary", () => {
  const scenes = [scene("A"), scene("B")];
  const alwaysSwap = () => 0;
  const next = buildNextSelectedQueueCycle(scenes, "B", alwaysSwap);

  assert.deepEqual(
    next.map(({ id }) => id),
    ["A", "B"]
  );
});

test("three-scene queues reject repeated order and boundary before accepting", () => {
  const scenes = [scene("A"), scene("B"), scene("C")];
  const random = sequenceRandom([
    0.99,
    0.99, // A-B-C: repeated order
    0,
    0.99, // C-B-A: repeats previous final C
    0,
    0, // B-C-A: valid
  ]);
  const next = buildNextSelectedQueueCycle(scenes, "C", random);

  assert.deepEqual(
    next.map(({ id }) => id),
    ["B", "C", "A"]
  );
});

test("three-scene queues have a deterministic valid fallback", () => {
  const scenes = [scene("A"), scene("B"), scene("C")];
  const identityOnly = () => 0.99;
  const next = buildNextSelectedQueueCycle(scenes, "C", identityOnly);

  assert.deepEqual(
    next.map(({ id }) => id),
    ["B", "C", "A"]
  );
});

test("filtered cycles reject an empty result", () => {
  assert.equal(
    isFilteredQueueCycleCandidateValid({
      previousFinalSceneID: "C",
      previousSeed: 1,
      nextSeed: 2,
      previousSceneIDs: ["A", "B", "C"],
      nextSceneIDs: [],
      canCompareEntireCycle: true,
    }),
    false
  );
});

test("filtered cycles reject boundary, seed, and complete-order repeats", () => {
  const base = {
    previousFinalSceneID: "C",
    previousSeed: 1,
    nextSeed: 2,
    previousSceneIDs: ["A", "B", "C"],
    nextSceneIDs: ["B", "C", "A"],
    canCompareEntireCycle: true,
  };

  assert.equal(
    isFilteredQueueCycleCandidateValid({
      ...base,
      nextSceneIDs: ["C", "A", "B"],
    }),
    false
  );
  assert.equal(
    isFilteredQueueCycleCandidateValid({ ...base, nextSeed: 1 }),
    false
  );
  assert.equal(
    isFilteredQueueCycleCandidateValid({
      ...base,
      nextSceneIDs: ["A", "B", "C"],
    }),
    false
  );
});

test("filtered cycles accept a fresh boundary-safe permutation", () => {
  assert.equal(
    isFilteredQueueCycleCandidateValid({
      previousFinalSceneID: "C",
      previousSeed: 1,
      nextSeed: 2,
      previousSceneIDs: ["A", "B", "C"],
      nextSceneIDs: ["B", "C", "A"],
      canCompareEntireCycle: true,
    }),
    true
  );
});

test("selected queue session state is persisted and cleared", () => {
  const values = new Map();
  const originalWindow = global.window;
  global.window = {
    sessionStorage: {
      setItem(key, value) {
        values.set(key, value);
      },
      removeItem(key) {
        values.delete(key);
      },
    },
  };

  try {
    persistSelectedQueueCycle(["7", "8", "9"]);
    assert.equal(values.size, 1);

    const state = JSON.parse([...values.values()][0]);
    assert.equal(state.version, 1);
    assert.deepEqual(state.sceneIDs, ["7", "8", "9"]);
    assert.equal(typeof state.updatedAt, "number");

    clearSelectedQueueCycle();
    assert.equal(values.size, 0);
  } finally {
    global.window = originalWindow;
  }
});
