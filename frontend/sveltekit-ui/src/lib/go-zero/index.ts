function isRecord(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null; }

/** 保留既有 facade 的兼容壳判断；HTTP 状态永远优先。 */
export function inspectGoZeroPayload(payload: unknown, httpOk: boolean): { ok: true; data: unknown } | { ok: false; payload: unknown } {
  if (!httpOk) return { ok: false, payload };
  if (isRecord(payload) && 'code' in payload && 'msg' in payload && 'data' in payload) {
    if (![0, '0', null, undefined].includes(payload.code as 0 | '0' | null | undefined)) return { ok: false, payload };
    return { ok: true, data: payload.data };
  }
  return { ok: true, data: payload };
}

export function readGoZeroPage(payload: unknown): { items: unknown[]; page: number; pageSize: number; total: number } | null {
  if (!isRecord(payload) || !Array.isArray(payload.data)) return null;
  const { page, pageSize, total } = payload;
  if (typeof page !== 'number' || !Number.isSafeInteger(page) || page < 1 ||
      typeof pageSize !== 'number' || !Number.isSafeInteger(pageSize) || pageSize < 1 ||
      typeof total !== 'number' || !Number.isSafeInteger(total) || total < 0) return null;
  return { items: payload.data, page, pageSize, total };
}

export function buildGoZeroQuery(params: Record<string, string | number | boolean | null | undefined>): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== null && value !== undefined && value !== '') query.set(key, String(value));
  }
  const encoded = query.toString();
  return encoded ? `?${encoded}` : '';
}
