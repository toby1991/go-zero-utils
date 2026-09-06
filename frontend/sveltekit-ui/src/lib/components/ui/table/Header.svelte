<script lang="ts">
  import type { HTMLAttributes } from 'svelte/elements';
  import { cn } from '../utils.js';
  let { class: className, children, ...rest }: HTMLAttributes<HTMLTableSectionElement> = $props();
</script>
<thead data-slot="table-header" {...rest} class={cn('[&_tr]:border-b', className)}>{@render children?.()}</thead>
