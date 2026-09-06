export type QueryPatch = Record<string, string | number | boolean | null | undefined>;

/** 仅拥有的参数参与 patch；别名可列入 keys 后删除，其他表的参数不受影响。 */
export function queryPatch<Query extends object>(query: Query, defaults: Query, keys: readonly string[] = Object.keys(defaults)): QueryPatch {
  const result: QueryPatch = {};
  for (const key of keys) {
    const value = query[key as keyof Query];
    result[key] = value === defaults[key as keyof Query] || value === null || value === undefined || value === ''
      ? null
      : typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean' ? value : null;
  }
  return result;
}

export function parsePage(value: string | null): number {
  if (value === null || !/^[1-9]\d*$/.test(value)) return 1;
  const page = Number(value);
  return Number.isSafeInteger(page) ? page : 1;
}

/** 一个路由共享一个协调器，串行 patch 始终基于最新 URL。 */
export function createTableUrlCoordinator(options: {
  getUrl: () => URL;
  navigate: (url: URL, options: { noScroll: true; keepFocus: true }) => Promise<unknown>;
}) {
  let generation = 0;
  let queue = Promise.resolve();
  return {
    patch(patch: QueryPatch): Promise<void> {
      const epoch = generation;
      const values = { ...patch };
      const task = queue.then(async () => {
        if (epoch !== generation) return;
        const current = options.getUrl();
        const next = new URL(current);
        for (const [key, value] of Object.entries(values)) {
          if (value === null || value === undefined || value === '') next.searchParams.delete(key);
          else next.searchParams.set(key, String(value));
        }
        if (next.href !== current.href) await options.navigate(next, { noScroll: true, keepFocus: true });
      });
      queue = task.catch(() => undefined);
      return task;
    },
    cancel() { ++generation; }
  };
}
