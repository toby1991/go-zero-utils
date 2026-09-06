import { writable } from 'svelte/store';
import type { RefreshResult, TableController, TableControllerOptions, TableQuery, TableScope, TableSnapshot } from './types.js';

/** 列表读取唯一入口：草稿不参与请求，写后 fresh 不复用写前的在途读取。 */
export function createDataTableController<Row, Query extends TableQuery>(options: TableControllerOptions<Row, Query>): TableController<Row, Query> {
  const normalize = (query: Query) => options.normalize ? options.normalize({ ...query }) : { ...query };
  const keyOf = options.key ?? ((query: Query) => JSON.stringify(Object.keys(query).sort().map((key) => [key, query[key as keyof Query]])));
  const initial = normalize(options.initialQuery);
  let value: TableSnapshot<Row, Query> = {
    query: initial, draft: { ...initial }, queryKey: keyOf(initial), scope: null,
    rows: [], status: 'idle', error: null, page: initial.page ?? 1,
    pageSize: options.pagination === false ? 0 : options.pagination.pageSize,
    total: 0, pagination: options.pagination !== false
  };
  const store = writable(value);
  let sequence = 0;
  let destroyed = false;
  let flight: Promise<RefreshResult> | undefined;
  let draftRevision = 0;
  type NavigationTicket = { key: string; scope: TableScope; revision: number; intent: 'filters' | 'page' };
  const navigationTickets = new Set<NavigationTicket>();
  const update = (patch: Partial<typeof value>) => { value = { ...value, ...patch }; store.set(value); };

  function refresh({ fresh = false }: { fresh?: boolean } = {}): Promise<RefreshResult> {
    const { queryKey, scope } = value;
    const result = (status: RefreshResult['status']): RefreshResult => ({ status, queryKey, scope });
    if (destroyed || scope === null) return Promise.resolve(result('superseded'));
    if (flight && !fresh) return flight;
    const request = ++sequence;
    const query = { ...value.query };
    update({ status: 'loading', error: null, rows: [] });
    const pending = Promise.resolve().then(() => {
      // 身份/查询可能在微任务开始前已改变，不能用新凭据发出旧作用域的请求。
      if (request !== sequence || destroyed) throw new Error('Superseded table request');
      return options.read(query);
    }).then((data) => {
      if (request !== sequence || destroyed) return result('superseded');
      if (options.pagination !== false && !('page' in data)) throw new Error('Expected paginated table result');
      update({ rows: data.rows, status: 'ready', error: null,
        ...('page' in data ? { page: data.page, pageSize: data.pageSize, total: data.total } : { total: data.rows.length }) });
      return result('success');
    }).catch((error: unknown) => {
      if (request !== sequence || destroyed) return result('superseded');
      let message = '列表读取失败，请重试';
      try { message = options.errorMessage(error); } catch { /* 错误展示失败不能改变 refresh 的非抛出契约。 */ }
      update({ rows: [], status: 'error', error: message });
      return result('error');
    }).finally(() => { if (request === sequence) flight = undefined; });
    flight = pending;
    return pending;
  }

  function sync(query: Query, scope: TableScope, { restoreDraft = false } = {}): Promise<RefreshResult> {
    if (destroyed) return Promise.resolve({ status: 'superseded', queryKey: value.queryKey, scope });
    const next = normalize(query);
    const key = keyOf(next);
    const changed = key !== value.queryKey || scope !== value.scope;
    if (restoreDraft || scope !== value.scope) navigationTickets.clear();
    if (!changed) {
      if (restoreDraft) update({ draft: { ...next } });
      return flight ?? Promise.resolve({ status: value.status === 'error' ? 'error' : value.status === 'ready' ? 'success' : 'superseded', queryKey: key, scope });
    }
    ++sequence;
    flight = undefined;
    const ticket = [...navigationTickets].reverse().find((item) => item.key === key && item.scope === scope);
    const keepNewDraft = !restoreDraft && scope === value.scope && ticket && (ticket.intent === 'page' || draftRevision > ticket.revision);
    update({ query: next, draft: keepNewDraft ? value.draft : { ...next }, queryKey: key, scope, rows: [], error: null, status: 'idle', page: next.page ?? 1, total: 0 });
    return refresh();
  }

  async function navigate(query: Query, intent: NavigationTicket['intent']): Promise<void> {
    if (destroyed || value.scope === null) return;
    const next = normalize(query);
    const revision = draftRevision;
    const ticket: NavigationTicket = { key: keyOf(next), scope: value.scope, revision, intent };
    navigationTickets.add(ticket);
    try {
      if (options.navigate) await options.navigate(next);
      else await sync(next, value.scope);
      // 同 URL 提交不会产生路由事件；仍需清除草稿，且不能覆盖等待期间的新输入。
      if (!destroyed && navigationTickets.has(ticket) && intent === 'filters' && ticket.scope === value.scope && ticket.key === value.queryKey && revision === draftRevision) update({ draft: { ...next } });
    } finally {
      navigationTickets.delete(ticket);
    }
  }

  return {
    subscribe: store.subscribe,
    sync,
    patchDraft(patch) { if (!destroyed) { ++draftRevision; update({ draft: { ...value.draft, ...patch } }); } },
    submit() { return navigate(options.pagination === false ? value.draft : { ...value.draft, page: 1 }, 'filters'); },
    clear() { return navigate(options.pagination === false ? options.defaults : { ...options.defaults, page: 1 }, 'filters'); },
    goToPage(page) { return navigate({ ...value.query, page }, 'page'); },
    refresh,
    destroy() { destroyed = true; ++sequence; flight = undefined; navigationTickets.clear(); update({ rows: [], scope: null, status: 'idle', error: null }); }
  };
}
