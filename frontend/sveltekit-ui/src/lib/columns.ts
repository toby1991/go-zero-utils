import { createColumnHelper, rowPaginationFeature, tableFeatures as features, type RowData } from '@tanstack/svelte-table';

/** 只登记服务端分页；不得在当前页数据上再次切片、过滤或排序。 */
export type TableColumnMeta = { align?: 'left' | 'right' | 'center'; truncate?: boolean; width?: string };
export const tableFeatures = features({ rowPaginationFeature, columnMeta: {} as TableColumnMeta });
// TanStack columns() intentionally erases heterogeneous accessor values; retain its exact return type.
export type TableColumn<Row extends RowData> = ReturnType<ReturnType<typeof createColumnHelper<typeof tableFeatures, Row>>['columns']>[number];
export function createDataTableColumnHelper<Row extends RowData>() {
  return createColumnHelper<typeof tableFeatures, Row>();
}
export { renderSnippet, renderComponent } from '@tanstack/svelte-table';
