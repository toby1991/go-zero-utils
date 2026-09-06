import type { Readable } from 'svelte/store';

export type TableQuery = { page?: number };
export type TableResult<Row> = { rows: Row[] } | { rows: Row[]; page: number; pageSize: number; total: number };
export type TableScope = string | number | null;
export type RefreshResult = { status: 'success' | 'error' | 'superseded'; queryKey: string; scope: TableScope };
export type TableSnapshot<Row, Query extends TableQuery> = {
  query: Query; draft: Query; queryKey: string; scope: TableScope;
  rows: Row[]; status: 'idle' | 'loading' | 'ready' | 'error'; error: string | null;
  page: number; pageSize: number; total: number; pagination: boolean;
};
export type TableController<Row, Query extends TableQuery> = Readable<TableSnapshot<Row, Query>> & {
  sync(query: Query, scope: TableScope, options?: { restoreDraft?: boolean }): Promise<RefreshResult>;
  patchDraft(patch: Partial<Query>): void;
  submit(): Promise<void>;
  clear(): Promise<void>;
  goToPage(page: number): Promise<void>;
  refresh(options?: { fresh?: boolean }): Promise<RefreshResult>;
  destroy(): void;
};
export type TableControllerOptions<Row, Query extends TableQuery> = {
  initialQuery: Query;
  defaults: Query;
  key?: (query: Query) => string;
  normalize?: (query: Query) => Query;
  read: (query: Query) => Promise<TableResult<Row>>;
  errorMessage: (error: unknown) => string;
  pagination: false | { pageSize: number };
  navigate?: (query: Query) => Promise<unknown>;
};
export type FilterField<Query> = {
  id?: string;
  inputmode?: 'none' | 'text' | 'decimal' | 'numeric' | 'tel' | 'search' | 'email' | 'url';
  format?: (value: Query[Extract<keyof Query, string>]) => string;
  key: Extract<keyof Query, string>; label: string; placeholder?: string;
  type?: 'text' | 'select'; options?: readonly { label: string; value: string }[];
  parse?: (value: string) => Query[Extract<keyof Query, string>];
};
