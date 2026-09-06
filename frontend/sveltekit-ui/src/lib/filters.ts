import type { FilterField } from './types.js';
export function textFilter<Query>(key: Extract<keyof Query, string>, label: string, placeholder?: string): FilterField<Query> {
  return { key, label, placeholder, type: 'text' };
}
export function enumFilter<Query>(key: Extract<keyof Query, string>, label: string, options: readonly { label: string; value: string }[]): FilterField<Query> {
  return { key, label, type: 'select', options };
}
export function positiveId(value: string): number {
  if (!/^[1-9]\d*$/.test(value)) return 0;
  const id = Number(value);
  return Number.isSafeInteger(id) ? id : 0;
}
export function triStateBoolean(value: string): boolean | undefined {
  return value === 'true' ? true : value === 'false' ? false : undefined;
}
