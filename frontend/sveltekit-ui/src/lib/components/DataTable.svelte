<script lang="ts" generics="Row extends RowData, Query extends TableQuery">
  import { createTable, FlexRender, type RowData } from '@tanstack/svelte-table';
  import type { Snippet } from 'svelte';
  import { tableFeatures, type TableColumn } from '../columns.js';
  import type { FilterField, TableController, TableQuery } from '../types.js';
  import Button from './ui/Button.svelte';
  import Input from './ui/Input.svelte';
  import NativeSelect from './ui/NativeSelect.svelte';
  import * as Table from './ui/table/index.js';

  let {
    controller, columns, getRowId, filters = [], filterContent, toolbar,
    emptyMessage = '暂无数据', minWidth = '640px', ariaLabel = '数据列表', refreshLabel = '刷新'
  }: {
    controller: TableController<Row, Query>;
    columns: TableColumn<Row>[];
    getRowId: (row: Row) => string;
    filters?: FilterField<Query>[];
    filterContent?: Snippet<[{ draft: Query; patch: (patch: Partial<Query>) => void }]>;
    toolbar?: Snippet;
    emptyMessage?: string;
    minWidth?: string;
    ariaLabel?: string;
    refreshLabel?: string;
  } = $props();

  const fieldPrefix = $props.id();
  let navigationError = $state('');
  const snapshot = $derived($controller);
  const busy = $derived(snapshot.status === 'loading' || snapshot.status === 'idle');
  const hasFilters = $derived(filters.length > 0 || !!filterContent);
  const pageCount = $derived(Math.max(1, Math.ceil(snapshot.total / snapshot.pageSize)));
  const outOfRange = $derived(snapshot.pagination && snapshot.page > pageCount);
  const table = createTable({
    features: tableFeatures,
    get data() { return snapshot.status === 'ready' ? snapshot.rows : []; },
    get columns() { return columns; },
    getRowId: (row) => getRowId(row),
    manualPagination: true,
    autoResetPageIndex: false,
    get rowCount() { return snapshot.total; },
    state: {
      get pagination() { return { pageIndex: snapshot.page - 1, pageSize: snapshot.pageSize }; }
    }
  });

  // 导航失败不改已提交查询，也不把它冒充读取接口失败。
  async function navigate(action: () => Promise<void>) {
    navigationError = '';
    try { await action(); } catch { navigationError = '页面切换失败，请重试'; }
  }
  function patch(field: FilterField<Query>, value: string) {
    controller.patchDraft({ [field.key]: field.parse ? field.parse(value) : value } as Partial<Query>);
  }
</script>

<section aria-label={ariaLabel} class="min-w-0 space-y-4" data-slot="data-table">
  <form class="flex flex-wrap items-end gap-3" onsubmit={(event) => { event.preventDefault(); void navigate(() => controller.submit()); }}>
    {#each filters as field (field.key)}
      {@const id = field.id ?? `${fieldPrefix}-${field.key}`}
      <div class="w-full min-w-0 space-y-1.5 sm:w-48">
        <label for={id} class="text-sm font-medium text-foreground">{field.label}</label>
        {#if field.type === 'select'}
          <NativeSelect {id} name={field.key} value={field.format ? field.format(snapshot.draft[field.key]) : String(snapshot.draft[field.key] ?? '')} onchange={(event) => patch(field, event.currentTarget.value)}>
            {#each field.options ?? [] as option (option.value)}<option value={option.value}>{option.label}</option>{/each}
          </NativeSelect>
        {:else}
          <Input {id} name={field.key} inputmode={field.inputmode} placeholder={field.placeholder} value={field.format ? field.format(snapshot.draft[field.key]) : String(snapshot.draft[field.key] ?? '')} oninput={(event) => patch(field, event.currentTarget.value)} />
        {/if}
      </div>
    {/each}
    {@render filterContent?.({ draft: snapshot.draft, patch: (value) => controller.patchDraft(value) })}
    <div class="flex flex-wrap items-center gap-2">
      {#if hasFilters}
        <Button type="submit" variant="default">搜索</Button>
        <Button onclick={() => void navigate(() => controller.clear())}>清除筛选</Button>
      {/if}
      <Button disabled={busy} aria-label={refreshLabel} onclick={() => { void controller.refresh(); }}>{busy ? '加载中…' : refreshLabel}</Button>
    </div>
    {@render toolbar?.()}
  </form>
  {#if navigationError}<p role="alert" class="text-sm text-destructive">{navigationError}</p>{/if}
  <div class="min-w-0 rounded-md border border-border" aria-busy={busy}>
    <Table.Root aria-label={ariaLabel} style={`min-width: ${minWidth}`}>
      <Table.Header>
        {#each table.getHeaderGroups() as group (group.id)}
          <Table.Row>
            {#each group.headers as header (header.id)}
              <Table.Head colspan={header.colSpan} scope="col" style={`text-align: ${header.column.columnDef.meta?.align ?? 'left'}; width: ${header.column.columnDef.meta?.width ?? 'auto'}`}>
                {#if !header.isPlaceholder}<FlexRender {header} />{/if}
              </Table.Head>
            {/each}
          </Table.Row>
        {/each}
      </Table.Header>
      <Table.Body>
        {#if snapshot.status === 'ready'}
          {#each table.getRowModel().rows as row (row.id)}
            <Table.Row>
              {#each row.getAllCells() as cell (cell.id)}
                <Table.Cell style={`text-align: ${cell.column.columnDef.meta?.align ?? 'left'}`}>
                  {#if cell.column.columnDef.meta?.truncate}
                    <div class="max-w-xs truncate"><FlexRender {cell} /></div>
                  {:else}<FlexRender {cell} />{/if}
                </Table.Cell>
              {/each}
            </Table.Row>
          {/each}
        {/if}
      </Table.Body>
    </Table.Root>
    {#if busy}
      <div role="status" class="px-4 py-10 text-center text-sm text-muted-foreground">加载中…</div>
    {:else if snapshot.status === 'error'}
      <div class="space-y-3 px-4 py-10 text-center">
        <p role="alert" class="text-sm text-destructive">{snapshot.error}</p>
        <Button onclick={() => { void controller.refresh(); }}>重试</Button>
      </div>
    {:else if snapshot.rows.length === 0}
      <div class="space-y-3 px-4 py-10 text-center">
        <p role="status" class="text-sm text-muted-foreground">{outOfRange ? '当前页没有数据' : emptyMessage}</p>
        {#if outOfRange}<Button onclick={() => void navigate(() => controller.goToPage(1))}>回到第一页</Button>{/if}
      </div>
    {/if}
  </div>
  {#if snapshot.pagination}
    <nav aria-label={`${ariaLabel}分页`} class="flex flex-wrap items-center justify-between gap-3 text-sm">
      <p class="text-muted-foreground">共 {snapshot.total} 条 · 第 {snapshot.page} / {pageCount} 页</p>
      <div class="flex items-center gap-2">
        <Button disabled={busy || snapshot.page <= 1} onclick={() => void navigate(() => controller.goToPage(snapshot.page - 1))}>上一页</Button>
        <Button disabled={busy || snapshot.page >= pageCount} onclick={() => void navigate(() => controller.goToPage(snapshot.page + 1))}>下一页</Button>
      </div>
    </nav>
  {/if}
</section>
