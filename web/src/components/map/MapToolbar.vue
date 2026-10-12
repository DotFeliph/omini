<script setup lang="ts">
import {
  ArrowDownFromLine,
  ArrowRightFromLine,
  Bell,
  Download,
  LayoutGrid,
  Menu,
  RefreshCw,
  SquareDashed,
  X,
} from 'lucide-vue-next'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { NetworkGroup } from '@/lib/networks'
import { formatAgo } from '@/lib/format'
import { prefs } from '@/lib/prefs'

/**
 * The map's toolbar: the menu (devices and filters), alerts, failing
 * integrations and last update on the left, the map's controls on the right.
 */
const props = defineProps<{
  summary: { problems: number }
  /** Open critical and warning alerts not dismissed. */
  attention: number
  generatedAt?: string
  now: number
  networkGroups: NetworkGroup[]
  exporting: boolean
  refreshing: boolean
}>()
const emit = defineEmits<{ export: []; resetLayout: []; refresh: [] }>()
const devicesOpen = defineModel<boolean>('devicesOpen', { required: true })
const netFilter = defineModel<string | undefined>('netFilter')
const drawing = defineModel<boolean>('drawing', { required: true })
const { t, locale } = useI18n()

/** The network the map highlights, shown next to the menu to clear it. */
const network = computed(() => props.networkGroups.find((g) => g.key === netFilter.value))
/** Something is left out of the map: the menu shows a dot. */
const filtered = computed(() => !!network.value || prefs.hideOffline)

function toggleDirection() {
  prefs.layoutDirection = prefs.layoutDirection === 'RIGHT' ? 'DOWN' : 'RIGHT'
}
</script>

<template>
  <div class="toolbar">
    <div class="chips">
      <button
        type="button"
        class="btn small icon menu"
        :class="{ active: devicesOpen }"
        data-test="map-menu"
        :aria-expanded="devicesOpen"
        :aria-label="t('mapMenu.open')"
        :title="t('mapMenu.open')"
        @click="devicesOpen = !devicesOpen"
      >
        <Menu :size="16" />
        <span v-if="filtered" class="dot" data-test="filtered" />
      </button>
      <span v-if="network" class="chip net" data-test="net-chip">
        {{ network.name }}
        <button
          type="button"
          class="clear"
          :aria-label="t('mapMenu.clearNetwork')"
          @click="netFilter = undefined"
        >
          <X :size="12" />
        </button>
      </span>
      <RouterLink v-if="attention" to="/alerts" class="chip problem" data-test="alerts-chip">
        <Bell :size="12" />
        {{ t('map.alerts', { n: attention }, attention) }}
      </RouterLink>
      <RouterLink v-if="summary.problems" to="/integrations" class="chip problem">
        {{ t('map.problems', summary.problems) }}
      </RouterLink>
      <span v-if="generatedAt" class="updated muted">
        {{ t('map.updated', { ago: formatAgo(generatedAt, locale, now) }) }}
      </span>
    </div>
    <div class="buttons">
      <button
        class="btn small"
        :title="prefs.layoutDirection === 'RIGHT' ? t('map.topDown') : t('map.leftToRight')"
        data-test="direction"
        @click="toggleDirection"
      >
        <component
          :is="prefs.layoutDirection === 'RIGHT' ? ArrowDownFromLine : ArrowRightFromLine"
          :size="15"
        />
        <span class="label">
          {{ prefs.layoutDirection === 'RIGHT' ? t('map.topDown') : t('map.leftToRight') }}
        </span>
      </button>
      <button
        class="btn small"
        :class="{ active: drawing }"
        data-test="new-area"
        :aria-pressed="drawing"
        :title="t('map.areas.drawHint')"
        @click="drawing = !drawing"
      >
        <SquareDashed :size="15" /><span class="label">{{ t('map.areas.new') }}</span>
      </button>
      <button
        class="btn small"
        data-test="export"
        :disabled="exporting"
        aria-haspopup="dialog"
        @click="emit('export')"
      >
        <Download :size="15" /><span class="label">{{ t('map.export') }}</span>
      </button>
      <button class="btn small" :title="t('map.resetLayout')" @click="emit('resetLayout')">
        <LayoutGrid :size="15" /><span class="label">{{ t('map.resetLayout') }}</span>
      </button>
      <button class="btn small" :disabled="refreshing" @click="emit('refresh')">
        <RefreshCw :size="15" :class="{ spin: refreshing }" />
        <span class="label">{{ t('map.refresh') }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.toolbar {
  position: absolute;
  top: 12px;
  left: 12px;
  right: 12px;
  z-index: 4;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  pointer-events: none;
  flex-wrap: wrap;
}
.toolbar > * {
  pointer-events: auto;
}
.chips,
.buttons {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.chip {
  padding: 5px 12px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--bg-elevated);
  font-size: 13px;
  font-weight: 500;
  text-decoration: none;
  color: var(--text);
}
.menu {
  position: relative;
}
.dot {
  position: absolute;
  top: 3px;
  right: 3px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--accent);
}
.chip.net {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-color: var(--accent);
  color: var(--accent);
}
.clear {
  display: inline-flex;
  padding: 0;
  border: 0;
  background: none;
  color: inherit;
  cursor: pointer;
}
.chip.problem {
  background: var(--danger-soft);
  border-color: transparent;
  color: var(--danger);
}
.updated {
  font-size: 12.5px;
  margin-left: 4px;
}
.spin {
  animation: spin 0.8s linear infinite;
}
.btn.active {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--accent);
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 640px) {
  .label {
    display: none;
  }
}
</style>
