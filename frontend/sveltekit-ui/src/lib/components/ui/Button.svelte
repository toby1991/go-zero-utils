<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import { cn } from './utils.js';
  let { class: className, children, type = 'button', variant = 'outline', ...rest }: HTMLButtonAttributes & { variant?: 'outline' | 'default' } = $props();
</script>
<button data-slot="button" {type} {...rest} class={cn('inline-flex h-9 shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-md px-3 text-sm font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50', variant === 'default' ? 'bg-primary text-primary-foreground hover:bg-primary/90' : 'border border-input bg-background text-foreground hover:bg-accent hover:text-accent-foreground', className)}>{@render children?.()}</button>
