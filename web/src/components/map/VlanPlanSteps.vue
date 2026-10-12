<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import type { Network, PortVlans, VlanPlan } from '@/lib/types'
import { describeVlans } from '@/lib/vlans'

/** What a VLAN plan changes, device by device, and what is left to do by hand. */
const props = defineProps<{
  plan: VlanPlan
  /** A node's name on the map. */
  nameOf: (id: string) => string
}>()
const { t } = useI18n()

function stepText(s: VlanPlan['steps'][number]) {
  const who = props.nameOf(s.node_id)
  if (s.kind === 'vlan.set') {
    const v = s.value as { id: number; name?: string }
    return t('portVlans.stepCreate', { device: who, vlan: v.name ? `${v.id} · ${v.name}` : v.id })
  }
  if (s.kind === 'network.set') {
    const n = s.value as Network
    return t('portVlans.stepNetwork', { device: who, vlan: n.vlan, gateway: n.gateway ?? '' })
  }
  return t('portVlans.stepPort', {
    device: who,
    port: s.port,
    vlans: describeVlans(s.value as PortVlans, t),
  })
}
function noteText(n: VlanPlan['notes'][number]) {
  return t(`portVlans.notes.${n.reason}`, {
    device: props.nameOf(n.node_id),
    vlans: (n.vlans ?? []).join(', '),
  })
}
</script>

<template>
  <ol class="steps" data-test="vlan-plan">
    <li v-for="(s, i) in plan.steps" :key="i" :class="{ danger: s.protected }">
      {{ stepText(s) }}
    </li>
  </ol>
  <ul v-if="plan.notes.length" class="notes" data-test="vlan-notes">
    <li v-for="(n, i) in plan.notes" :key="i">{{ noteText(n) }}</li>
  </ul>
</template>

<style scoped>
.steps,
.notes {
  display: grid;
  gap: 4px;
  margin: 0;
  padding-left: 18px;
  font-size: 12.5px;
}
.steps .danger {
  color: var(--danger);
}
.notes {
  color: var(--text-muted);
}
</style>
