const SELECTED_QUEUE_CYCLE_STORAGE_KEY = "homeStash.selectedQueueCycle.v1";

interface ISelectedQueueCycleState {
  version: 1;
  sceneIDs: string[];
  updatedAt: number;
}

function sameSceneOrder<T extends { id: string }>(
  first: readonly T[],
  second: readonly T[]
) {
  return (
    first.length === second.length &&
    first.every((scene, index) => scene.id === second[index].id)
  );
}

export function fisherYatesShuffle<T>(
  values: readonly T[],
  random: () => number = Math.random
) {
  const shuffled = [...values];

  for (let index = shuffled.length - 1; index > 0; index -= 1) {
    const randomIndex = Math.floor(random() * (index + 1));
    [shuffled[index], shuffled[randomIndex]] = [
      shuffled[randomIndex],
      shuffled[index],
    ];
  }

  return shuffled;
}

export function buildNextSelectedQueueCycle<T extends { id: string }>(
  scenes: readonly T[],
  previousFinalSceneID: string,
  random: () => number = Math.random
) {
  if (scenes.length <= 1) {
    return [...scenes];
  }

  const maximumAttempts = Math.max(12, scenes.length * 2);

  for (let attempt = 0; attempt < maximumAttempts; attempt += 1) {
    const candidate = fisherYatesShuffle(scenes, random);
    const repeatsBoundary = candidate[0].id === previousFinalSceneID;
    const repeatsOrder =
      scenes.length >= 3 && sameSceneOrder(candidate, scenes);

    if (!repeatsBoundary && !repeatsOrder) {
      return candidate;
    }
  }

  // Two-item queues must keep their order at a completed-cycle boundary:
  // A → B must restart at A, otherwise B would repeat across the boundary.
  if (scenes.length === 2) {
    return [...scenes];
  }

  // Deterministic fallback for the extremely unlikely case where random
  // attempts did not produce a valid order. A one-position rotation changes
  // the order and cannot start with the previous final item for unique IDs.
  return [...scenes.slice(1), scenes[0]];
}

interface IFilteredQueueCycleCandidate {
  previousFinalSceneID: string;
  previousSeed: number;
  nextSeed: number;
  previousSceneIDs: readonly string[];
  nextSceneIDs: readonly string[];
  canCompareEntireCycle: boolean;
}

export function isFilteredQueueCycleCandidateValid({
  previousFinalSceneID,
  previousSeed,
  nextSeed,
  previousSceneIDs,
  nextSceneIDs,
  canCompareEntireCycle,
}: IFilteredQueueCycleCandidate) {
  if (nextSceneIDs.length === 0) return false;

  const repeatsBoundary = nextSceneIDs[0] === previousFinalSceneID;
  const repeatsSeed = previousSeed !== -1 && nextSeed === previousSeed;
  const repeatsEntireOrder =
    nextSceneIDs.length >= 3 &&
    canCompareEntireCycle &&
    previousSceneIDs.length === nextSceneIDs.length &&
    previousSceneIDs.every((sceneID, index) => sceneID === nextSceneIDs[index]);

  return !repeatsBoundary && !repeatsSeed && !repeatsEntireOrder;
}

export function persistSelectedQueueCycle(sceneIDs: readonly string[]) {
  if (typeof window === "undefined") return;

  const state: ISelectedQueueCycleState = {
    version: 1,
    sceneIDs: [...sceneIDs],
    updatedAt: Date.now(),
  };

  try {
    window.sessionStorage.setItem(
      SELECTED_QUEUE_CYCLE_STORAGE_KEY,
      JSON.stringify(state)
    );
  } catch {
    // Playback must continue even when browser storage is unavailable.
  }
}

export function clearSelectedQueueCycle() {
  if (typeof window === "undefined") return;

  try {
    window.sessionStorage.removeItem(SELECTED_QUEUE_CYCLE_STORAGE_KEY);
  } catch {
    // Playback must continue even when browser storage is unavailable.
  }
}
