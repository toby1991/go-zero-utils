<script lang="ts">
  import type { HTMLAttributes } from 'svelte/elements';
  import { cn } from '../utils.js';
  let { class: className, children, ...rest }: HTMLAttributes<HTMLTableRowElement> = $props();
</script>
<tr data-slot="table-row" {...rest} class={cn('border-b border-border transition-colors hover:bg-muted/50', className)}>{@render children?.()}</tr>
