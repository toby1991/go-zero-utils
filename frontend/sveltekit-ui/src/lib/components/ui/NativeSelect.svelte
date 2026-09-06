<script lang="ts">
  import type { HTMLSelectAttributes } from 'svelte/elements';
  import { ChevronDown } from 'lucide-svelte';
  import { cn } from './utils.js';
  let { class: className, children, value = $bindable(), ...rest }: HTMLSelectAttributes = $props();
</script>
<div data-slot="native-select-wrapper" class="relative w-full">
  <select data-slot="native-select" bind:value {...rest} class={cn('h-9 w-full appearance-none rounded-md border border-input bg-background py-1 pl-3 pr-8 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50', className)}>{@render children?.()}</select>
  <ChevronDown class="pointer-events-none absolute top-1/2 right-2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
</div>
