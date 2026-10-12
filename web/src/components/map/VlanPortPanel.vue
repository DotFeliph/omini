<script setup lang="ts">
import { Shield } from 'lucide-vue-next'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api, ApiError } from '@/lib/api'
import type { Interface, PortVlans, TopoNode, VlanPlan } from '@/lib/types'
import { describeVlans } from '@/lib/vlans'

import VlanPlanSteps from './VlanPlanSteps.vue'

/**
 * A VLAN's ports: the front of every switch Omini can change. A click on a
 * port moves it out → in the VLAN (access) → tagged (trunk) → out; the plan
 * (the way to the router too) is shown before it is applied.
 */
const props = defineProps<{
  vlan: number
  /** The map's nodes, by id. */
  nodes: Map<string, TopoNode>
  nameOf: (id: string) => string
  /** Destructive changes are allowed (Settings). */
  allowDestructive: boolean
}>()
const emit = defineEmits<{ applied: [] }>()
const { t } = useI18n()

type State = 'out' | 'in' | 'tagged'

const switches = computed(() =>
  [...props.nodes.values()]
    .filter((n) => n.changes?.includes('port.vlans'))
    .map((n) => ({
      node: n,
      ports: (n.device?.interfaces ?? [])
        .filter((i) => i.vlans)
        .sort((a, b) => portNumber(a.name) - portNumber(b.name) || a.name.localeCompare(b.name)),
    }))
    .filter((s) => s.ports.length)
    .sort((a, b) => props.nameOf(a.node.id).localeCompare(props.nameOf(b.node.id))),
)

function portNumber(name: string) {
  const m = /(\d+)$/.exec(name)
  return m ? Number(m[1]) : Infinity
}
/** "Port 9" → "9", "ether2" → "ether2". */
const short = (name: string) => /^port\s*(\d+)$/i.exec(name)?.[1] ?? name

function stateOf(p: Interface): State {
  if (p.vlans?.untagged === props.vlan) return 'in'
  if (p.vlans?.tagged?.includes(props.vlan)) return 'tagged'
  return 'out'
}
const NEXT: Record<State, State> = { out: 'in', in: 'tagged', tagged: 'out' }

/** The port's VLANs once it is in the next state. */
function want(p: Interface, to: State): PortVlans {
  const cur = p.vlans ?? {}
  const others = (cur.tagged ?? []).filter((v) => v !== props.vlan)
  if (to === 'in') return { untagged: props.vlan }
  if (to === 'tagged') {
    const native = cur.untagged === props.vlan ? 1 : (cur.untagged ?? 1)
    return { untagged: native, tagged: [...others, props.vlan].sort((a, b) => a - b) }
  }
  const native = cur.untagged === props.vlan ? 1 : cur.untagged
  return native === undefined && !others.length
    ? { untagged: 1 }
    : { untagged: native, tagged: others }
}

const pending = ref<{
  node: string
  port: Interface
  to: State
  want: PortVlans
  plan?: VlanPlan
}>()
const busy = ref(false)
const error = ref('')
const again = ref(false)
const destructive = computed(() => pending.value?.plan?.steps.some((s) => s.protected) ?? false)

async function click(node: TopoNode, p: Interface) {
  const to = NEXT[stateOf(p)]
  const w = want(p, to)
  pending.value = { node: node.id, port: p, to, want: w }
  again.value = false
  error.value = ''
  busy.value = true
  try {
    pending.value.plan = await api.planVlans(node.id, p.name, w)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : t('common.error')
  } finally {
    busy.value = false
  }
}

async function apply() {
  const p = pending.value
  if (!p?.plan) return
  if (destructive.value && !again.value) {
    again.value = true
    return
  }
  busy.value = true
  error.value = ''
  try {
    await api.applyVlans(p.node, p.port.name, p.want, destructive.value)
    pending.value = undefined
    emit('applied')
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : t('common.error')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="ports" data-test="vlan-ports">
    <p v-if="!switches.length" class="muted small">{{ t('vlanPorts.none') }}</p>
    <div v-for="s in switches" :key="s.node.id" class="switch" :data-switch="s.node.id">
      <div class="switch-name">{{ nameOf(s.node.id) }}</div>
      <div class="grid">
        <button
          v-for="p in s.ports"
          :key="p.name"
          type="button"
          class="port"
          :class="[
            stateOf(p),
            { pending: pending?.node === s.node.id && pending.port.name === p.name },
          ]"
          :title="`${p.name}${p.description ? ' · ' + p.description : ''} — ${t(`vlanPorts.state.${stateOf(p)}`)}`"
          :disabled="busy"
          :data-port="p.name"
          @click="click(s.node, p)"
        >
          {{ short(p.name) }}
          <Shield v-if="s.node.protected?.[p.name]" :size="9" class="guard" />
        </button>
      </div>
    </div>
    <div v-if="switches.length" class="legend muted small">
      <span><i class="dot in" />{{ t('vlanPorts.state.in') }}</span>
      <span><i class="dot tagged" />{{ t('vlanPorts.state.tagged') }}</span>
      <span><i class="dot out" />{{ t('vlanPorts.state.out') }}</span>
      <span>{{ t('vlanPorts.hint') }}</span>
    </div>

    <div v-if="pending" class="plan" data-test="port-plan">
      <p class="small">
        {{
          t('vlanPorts.change', {
            device: nameOf(pending.node),
            port: pending.port.name,
            state: t(`vlanPorts.state.${pending.to}`),
            vlans: describeVlans(pending.want, t),
          })
        }}
      </p>
      <VlanPlanSteps v-if="pending.plan" :plan="pending.plan" :name-of="nameOf" />
      <p v-if="destructive && !allowDestructive" class="error small">
        {{ t('portVlans.blocked') }}
      </p>
      <p v-else-if="again" class="warn small">{{ t('portVlans.askAgain') }}</p>
      <div class="actions">
        <button class="btn small ghost" type="button" :disabled="busy" @click="pending = undefined">
          {{ t('common.cancel') }}
        </button>
        <button
          class="btn small"
          :class="destructive ? 'danger' : 'primary'"
          type="button"
          data-test="port-apply"
          :disabled="busy || !pending.plan || (destructive && !allowDestructive)"
          @click="apply"
        >
          {{
            busy ? t('changes.applying') : again ? t('portVlans.applyAnyway') : t('portVlans.apply')
          }}
        </button>
      </div>
    </div>
    <p v-if="error" class="error small" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.ports {
  display: grid;
  gap: 12px;
  margin-top: 10px;
}
.switch-name {
  margin-bottom: 6px;
  font-size: 12.5px;
  font-weight: 600;
}
.grid {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.port {
  position: relative;
  min-width: 34px;
  height: 30px;
  padding: 0 6px;
  border: 1px solid var(--border-strong);
  border-radius: 5px;
  background: var(--surface);
  color: var(--text-muted);
  font: inherit;
  font-family: var(--mono);
  font-size: 11.5px;
  cursor: pointer;
}
.port.in {
  border-color: var(--net, var(--accent));
  background: var(--net, var(--accent));
  color: #fff;
  font-weight: 700;
}
.port.tagged {
  border: 2px solid var(--net, var(--accent));
  color: var(--text);
  font-weight: 700;
}
.port.pending {
  outline: 2px dashed var(--warn);
  outline-offset: 2px;
}
.guard {
  position: absolute;
  top: 1px;
  right: 1px;
  opacity: 0.7;
}
.legend {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}
.dot {
  display: inline-block;
  width: 10px;
  height: 10px;
  margin-right: 5px;
  border-radius: 3px;
  vertical-align: -1px;
}
.dot.in {
  background: var(--net, var(--accent));
}
.dot.tagged {
  border: 2px solid var(--net, var(--accent));
}
.dot.out {
  border: 1px solid var(--border-strong);
}
.plan {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  background: var(--surface);
}
.actions {
  display: flex;
  gap: 6px;
  justify-content: flex-end;
}
.small {
  margin: 0;
  font-size: 12.5px;
}
.warn {
  color: var(--warn);
}
.error {
  color: var(--danger);
}
</style>
