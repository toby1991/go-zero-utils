<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';
  import { cn } from './utils.js';
  let { class: className, type = 'text', ...rest }: HTMLInputAttributes = $props();
</script>
<input data-slot="input" {type} {...rest} class={cn('flex h-9 w-full min-w-0 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50', className)} />
