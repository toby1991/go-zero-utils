<script lang="ts">
  import type { HTMLThAttributes } from 'svelte/elements';
  import { cn } from '../utils.js';
  let { class: className, children, ...rest }: HTMLThAttributes = $props();
</script>
<th data-slot="table-head" {...rest} class={cn('h-11 px-4 text-left align-middle font-medium whitespace-nowrap text-muted-foreground', className)}>{@render children?.()}</th>
