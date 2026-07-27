import type { ListFilterModel } from "src/models/list-filter/filter";

export const HOME_STASH_TV_MAX_SCENES = 500;
const PAGE_SIZE = 100;

interface IScenePage {
  data: {
    findScenes: {
      count: number;
      scenes: { id: string }[];
    };
  };
}

type ScenePageQuery = (filter: ListFilterModel) => Promise<IScenePage>;

export async function resolveFilteredSceneIDs(
  filter: ListFilterModel,
  queryPage: ScenePageQuery
) {
  const pageFilter = filter.clone();
  pageFilter.currentPage = 1;
  pageFilter.itemsPerPage = PAGE_SIZE;

  const sceneIDs: string[] = [];
  let total = 0;

  do {
    const result = await queryPage(pageFilter);
    const { count, scenes } = result.data.findScenes;
    total = count;

    if (total > HOME_STASH_TV_MAX_SCENES) {
      throw new Error(
        `This filtered queue contains ${total} scenes. Narrow it to ${HOME_STASH_TV_MAX_SCENES} or fewer before sending.`
      );
    }

    sceneIDs.push(...scenes.map((scene) => scene.id));
    pageFilter.currentPage += 1;
  } while (sceneIDs.length < total);

  if (!sceneIDs.length) {
    throw new Error("The filtered queue contains no scenes.");
  }

  return sceneIDs;
}
