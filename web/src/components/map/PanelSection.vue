<script setup lang="ts">
import { ChevronRight } from 'lucide-vue-next'
import { computed } from 'vue'

import { prefs } from '@/lib/prefs'

/**
 * A section of the device panel: folded to its title and a short summary
 * until clicked. Which sections are open is remembered (per browser), for
 * every device. Content stays in the page while folded; the default slot gets
 * `open` for what should load only when shown (a chart).
 */
const props = defineProps<{
  /** Remembers the section open or folded. */
  id: string
  title: string
  /** Shown next to the title while folded, e.g. "V200.1.7 · 49d". */
  summary?: string
  /** Always open, without a fold (the panel's main sections). */
  fixed?: boolean
}>()

const open = computed(() => props.fixed || prefs.openSections.includes(props.id))
function toggle() {
  prefs.openSections = open.value
    ? prefs.openSections.filter((s) => s !== props.id)
    : [...prefs.openSections, props.id]
}
</script>

<template>
  <section class="block" :class="{ folded: !open }" :data-section="id">
    <h3 v-if="fixed">{{ title }}</h3>
    <button
      v-else
      type="button"
      class="fold"
      :aria-expanded="open"
      :data-test="`fold-${id}`"
      @click="toggle"
    >
      <ChevronRight :size="14" class="chevron" :class="{ open }" />
      <span class="title">{{ title }}</span>
      <span v-if="summary && !open" class="summary">{{ summary }}</span>
    </button>
    <div v-show="open" class="content">
      <slot :open="open" />
    </div>
  </section>
</template>

<style scoped>
.block {
  padding-top: 12px;
  margin: 14px 0 0;
  border-top: 1px solid var(--border);
}
.block.folded {
  padding-top: 8px;
  margin-top: 8px;
}
h3,
.title {
  margin: 0;
  font-size: 13px;
  color: var(--text-muted);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
h3 {
  margin-bottom: 10px;
}
.fold {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 2px 0;
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.fold:hover .title,
.fold:focus-visible .title {
  color: var(--text);
}
.chevron {
  flex: none;
  color: var(--text-muted);
  transition: transform 0.15s;
}
.chevron.open {
  transform: rotate(90deg);
}
.summary {
  margin-left: auto;
  overflow: hidden;
  color: var(--text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.content {
  margin-top: 10px;
}
</style>
