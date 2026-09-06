<script lang="ts">
  import type { HTMLAttributes } from 'svelte/elements';
  import { cn } from '../utils.js';
  let { class: className, children, ...rest }: HTMLAttributes<HTMLTableSectionElement> = $props();
</script>
<tbody data-slot="table-body" {...rest} class={cn('[&_tr:last-child]:border-0', className)}>{@render children?.()}</tbody>
