# @toby1991/go-zero-utils-sveltekit-ui

Svelte 5 server-driven data tables using TanStack Table v9 and shadcn-svelte
primitives. The package has an independent npm version; it does not change
the enclosing Go module or require any Go runtime.

## Install and theme

```sh
bun add --exact @toby1991/go-zero-utils-sveltekit-ui@0.1.0
```

For Tailwind v4 applications with `src/app.css`, add this source declaration:

```css
@source "../node_modules/@toby1991/go-zero-utils-sveltekit-ui/dist";
```

The host supplies its shadcn theme variables (`background`, `foreground`,
`border`, `input`, `ring`, `primary`, `muted`, and their foreground variants).
There is no global reset. Svelte and SvelteKit are peer dependencies.

## Ownership and entry points

- Root: DataTable, column helpers, filter presets and a Svelte-readable controller.
- `/go-zero`: pure response/page/query helpers; no HTTP, authentication or Toast.
- `/sveltekit`: URL parsing and a serial query-patch coordinator; no business reads.

Keep row DTO validation, permission decisions, mutations, authentication and
safe error messages in the host. Never import a host `$lib` module into this package.

## Controller

```ts
import { createDataTableController } from '@toby1991/go-zero-utils-sveltekit-ui';

type Query = { page: number; code: string };
type Row = { id: string; code: string };
const defaults: Query = { page: 1, code: '' };
const controller = createDataTableController<Row, Query>({
  initialQuery: defaults,
  defaults,
  pagination: { pageSize: 10 },
  normalize: (query) => ({ ...query, code: query.code.trim() }),
  read: (query) => facade.list(query), // { rows, page, pageSize, total }
  errorMessage: () => '列表读取失败，请重试',
  navigate: (query) => coordinator.patch(queryPatch(query, defaults))
});
```

Bind `controller.sync(parsedQuery, sessionScope)` in one host adapter and destroy
the controller on unmount. A null scope clears rows and stops reads. Session
scope is an in-memory identity generation, never an access/refresh token; token
renewal alone must not change it. Route load parses queries, while the controller
is the sole list reader. Unrelated query parameters do not trigger reads.

The controller is a Svelte readable: `$controller` exposes rows, query, draft,
status, safe error, pagination and request identity. Filters edit `patchDraft`;
only `submit()` applies them. `clear()` resets defaults, `goToPage()` uses applied
filters. `refresh()` preserves drafts and joins the current flight.

After a successful mutation call `refresh({ fresh: true })`: it starts a new GET
even when a pre-write GET is in flight. It resolves `{status, queryKey, scope}`,
where status is `success`, `error`, or `superseded`; it never converts a read
failure into a mutation failure. Commit one-time secrets and success UI before
refreshing. If a write must return to page 1, change page when not already there;
the changed query starts a new read, so do not add a second refresh.

For non-paginated data use query `{}`, `pagination: false` and return `{ rows }`.
No page metadata is required from that reader.

## Columns and filters

```svelte
<script lang="ts">
  import { DataTable, createDataTableColumnHelper, renderSnippet } from '@toby1991/go-zero-utils-sveltekit-ui';
  const helper = createDataTableColumnHelper<Row>();
  const columns = helper.columns([
    helper.accessor('code', { header: '编号' }),
    helper.display({ id: 'actions', header: '操作', cell: ({ row }) => renderSnippet(actions, row.original) })
  ]);
</script>

{#snippet actions(row: Row)}<button type="button" onclick={() => edit(row)}>编辑</button>{/snippet}
<DataTable {controller} {columns} getRowId={(row) => row.id}
  filters={[{ key: 'code', label: '编号', id: 'code-filter' }]}
  ariaLabel="用户组" minWidth="720px" />
```

Use application button primitives inside business snippets. Built-in filters
support text and native select, `id`, `inputmode`, parse/format and options.
Use `filterContent` with `{ draft, patch }` for an existing domain picker; do not
implement another submit, URL or loading loop there. `toolbar` accepts additional
host actions. `refreshLabel` distinguishes independent dashboard tables.

There is no current-page sorting/filtering/slicing: all pagination is controlled
server-side. Use durable row IDs, not array indices. Loading and failed reads
remove writable old rows while preserving headers and filters. Empty out-of-range
pages provide a first-page action without clearing filters.

## Route coordination

Create one coordinator per route (not per dashboard panel):

```ts
import { createTableUrlCoordinator, parsePage, queryPatch } from '@toby1991/go-zero-utils-sveltekit-ui/sveltekit';
const coordinator = createTableUrlCoordinator({ getUrl: () => page.url, navigate: goto });
```

`patch()` merges only named keys into the latest URL, preserving other tables,
unrelated parameters and hash. `queryPatch(query, defaults, ownedKeys?)` omits
default values; include retired alias keys in ownedKeys to delete them.
Cancel queued patches on popstate or route exit, not ordinary same-route goto.
After popstate call `sync(query, scope, { restoreDraft: true })`, including when
the normalized query key did not change. Failed navigation leaves applied state
unchanged. Never use shallow routing or invalidateAll as another list read path.

## go-zero-utils preset

`inspectGoZeroPayload(payload, httpOk)` preserves existing facade compatibility:
complete code/msg/data envelopes accept 0, '0', null and undefined as success;
successful raw payloads pass through. HTTP failures always fail. Keep the original
transport status and host typed error/422 callback when the helper returns failure.

`readGoZeroPage` accepts the *inner* pagination object `{page,pageSize,total,data}`
and returns `{page,pageSize,total,items}` or null. Metadata must be safe integers;
invalid data is not an empty list. `buildGoZeroQuery` returns an encoded `?` suffix
or an empty string, omitting null/undefined/empty string while preserving false/0.
These are this repository's conventions, not universal go-zero defaults.

## Verify and release

```sh
bun install --frozen-lockfile
bun test
bun run check
bun run build
npm pack
npm pack --dry-run
```

Install the actual tarball into consumers, then run their type checks, builds and
browser suites. Publish only after those pass and npm scope ownership is verified.
Use npm version 0.1.0 and git tag `go-zero-utils-sveltekit-ui-v0.1.0`, independently
of Go tags. Consumer release dependencies must be exact registry versions, not
absolute file paths, copied sources or symlinks. Do not publish production fixtures
or credentials. This package's public files are allowlisted in package.json.

### Publication gate and commands

The commands below are release instructions, not evidence that this version has
been published. A local tarball and successful tests do not satisfy publication.
Run them from this package directory only after consumer acceptance and the
source commit have been recorded.

```sh
npm whoami --registry=https://registry.npmjs.org/
npm view @toby1991/go-zero-utils-sveltekit-ui@0.1.0 version --registry=https://registry.npmjs.org/
```

Confirm that the authenticated account is authorized to publish under `@toby1991`.
`ENEEDAUTH` blocks publication; log in through the maintainer's normal npm login
flow and retry the identity check. A missing package/version does not establish
scope ownership. If version 0.1.0 already exists, stop and compare its provenance;
never attempt to overwrite it. Do not put tokens or one-time passwords in the
repository, command history, screenshots or test artifacts.

Review `npm pack --dry-run` for the intended allowlist, then publish the exact
tarball that passed consumer acceptance, not an unverified live directory:

```sh
npm publish ./toby1991-go-zero-utils-sveltekit-ui-0.1.0.tgz --access public --registry=https://registry.npmjs.org/
npm view @toby1991/go-zero-utils-sveltekit-ui@0.1.0 version dist.integrity --registry=https://registry.npmjs.org/
```

Verify the returned version and integrity against the accepted tarball. If the
publish response is uncertain, inspect the registry result before retrying.
Only after publication is confirmed, tag the reviewed go-zero-utils source
commit (replace `VERIFIED_COMMIT_SHA` with that actual commit):

```sh
git tag -a go-zero-utils-sveltekit-ui-v0.1.0 VERIFIED_COMMIT_SHA -m "Release go-zero-utils-sveltekit-ui 0.1.0"
git push origin go-zero-utils-sveltekit-ui-v0.1.0
```

Do not move an existing tag. Update both consumers to exact registry version
`0.1.0`, remove temporary tarball dependencies, and rerun fresh-install checks,
builds and browser regressions. Go module tags remain independent. If credentials
or registry permissions are unavailable, report publication and registry-consumer
verification as pending; do not mark the release complete.
