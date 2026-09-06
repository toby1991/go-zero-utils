<script lang="ts">
  import type { HTMLTdAttributes } from 'svelte/elements';
  import { cn } from '../utils.js';
  let { class: className, children, ...rest }: HTMLTdAttributes = $props();
</script>
<td data-slot="table-cell" {...rest} class={cn('px-4 py-3 align-middle', className)}>{@render children?.()}</td>
