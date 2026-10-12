<script setup lang="ts">
import { Cable, Crosshair, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import ToggleSwitch from '@/components/ToggleSwitch.vue'
import VlanForm from '@/components/map/VlanForm.vue'
import VlanPortPanel from '@/components/map/VlanPortPanel.vue'
import { api, ApiError } from '@/lib/api'
import { displayName } from '@/lib/names'
import type { NetworkGroup } from '@/lib/networks'
import { prefs } from '@/lib/prefs'
import type { NetworkVlan, TopoNode } from '@/lib/types'
import { vlanHex } from '@/lib/vlans'

/**
 * The network's VLANs and subnets, a tab of the map's menu. A click on one
 * highlights it on the map and zooms to it. Omini keeps the VLANs and applies
 * them to the devices: create, rename and delete here; put ports in them from
 * a port's card.
 */
const props = defineProps<{
  /** The networks on the map (see lib/networks), for the filter. */
  groups: NetworkGroup[]
  /** Network key → color. */
  colors: Map<string, string>
  /** The network highlighted on the map. */
  picked?: string
}>()
const emit = defineEmits<{ select: [id: string]; pick: [key: string | undefined] }>()
const { t } = useI18n()

const vlans = ref<NetworkVlan[]>([])
const nodes = ref(new Map<string, TopoNode>())
const loading = ref(true)
const error = ref('')

const allowDestructive = ref(false)
async function load() {
  try {
    const [list, topo, settings] = await Promise.all([
      api.vlans(),
      api.topology(),
      api.changeSettings().catch(() => ({ allow_destructive: false })),
    ])
    vlans.value = list
    nodes.value = new Map(topo.topology.nodes.map((n) => [n.id, n]))
    allowDestructive.value = settings.allow_destructive
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : t('common.error')
  } finally {
    loading.value = false
  }
}
onMounted(load)

const name = (id: string) => {
  const n = nodes.value.get(id)
  return n ? displayName(n, t) : id
}
function open(id: string) {
  emit('select', id)
}

/** VLANs whose ports are open (the switches' front). */
const portsOpen = ref(new Set<number>())
function togglePorts(id: number) {
  const next = new Set(portsOpen.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  portsOpen.value = next
}

/** Access points Omini can configure: VLANs get a Wi-Fi network. */
const wifiCapable = computed(() =>
  [...nodes.value.values()].some((n) => n.changes?.includes('wifi.set')),
)

/** A router Omini can configure: VLANs get a network (gateway, DHCP, access). */
const router = computed(() =>
  [...nodes.value.values()].some((n) => n.changes?.includes('network.set')),
)

// One form serves to create and to edit (the id is fixed once created).
const editing = ref<number | 'new'>()
const busy = ref(false)
const editingVlan = computed(() =>
  typeof editing.value === 'number' ? vlans.value.find((v) => v.id === editing.value) : undefined,
)
function startNew() {
  editing.value = 'new'
  error.value = ''
}
function startEdit(v: NetworkVlan) {
  editing.value = v.id
  error.value = ''
}
async function saved() {
  editing.value = undefined
  await load()
}

/** How a VLAN's network reads on its card: gateway, DHCP range, access. */
function summary(v: NetworkVlan): string {
  const n = v.network
  if (!n?.gateway) return ''
  const parts = [t('vlans.net.gatewayShort', { ip: n.gateway.split('/')[0] })]
  parts.push(
    n.dhcp?.enabled
      ? t('vlans.net.dhcpRange', { from: n.dhcp.from, to: n.dhcp.to })
      : t('vlans.net.noDhcp'),
  )
  if (n.access) parts.push(t(`vlans.access.${n.access}`))
  if (n.mdns) parts.push(t('vlans.net.mdnsOn'))
  for (const w of v.wifi ?? [])
    parts.push(
      w.enabled ? t('vlans.wifi.short', { ssid: w.ssid }) : t('vlans.wifi.off', { ssid: w.ssid }),
    )
  return parts.join(' · ')
}

// Deleting takes a second click; it removes the VLAN from the router and
// every device first (a second confirmation when Omini is in it).
const deleting = ref<number>()
const deleteAgain = ref<number>()
/** Devices that lose their network with the VLAN. */
const losing = (v: NetworkVlan) => (v.network ? (groupOf(`vlan:${v.id}`)?.members.size ?? 0) : 0)
async function remove(v: NetworkVlan) {
  if (deleting.value !== v.id) {
    deleting.value = v.id
    return
  }
  busy.value = true
  error.value = ''
  try {
    await api.deleteVlan(v.id, deleteAgain.value === v.id)
    await load()
  } catch (e) {
    if (e instanceof ApiError && e.status === 409 && e.message === 'destructive') {
      if (allowDestructive.value) {
        deleteAgain.value = v.id
        busy.value = false
        return
      }
      error.value = t('vlans.net.blocked')
    } else {
      error.value = e instanceof ApiError ? e.message : t('common.error')
    }
  } finally {
    busy.value = false
    if (deleteAgain.value !== v.id) deleting.value = undefined
  }
}

const ports = (d: NetworkVlan['devices'][number]) =>
  [
    d.untagged.length ? t('vlans.untaggedPorts', { ports: d.untagged.join(', ') }) : '',
    d.tagged.length ? t('vlans.taggedPorts', { ports: d.tagged.join(', ') }) : '',
  ]
    .filter(Boolean)
    .join(' · ')
const sorted = computed(() => [...vlans.value].sort((a, b) => a.id - b.id))

/** A network on the map: a click highlights it (a click on the highlighted one clears it). */
const onMap = (key: string) => props.groups.some((g) => g.key === key)
function pick(key: string) {
  if (onMap(key)) emit('pick', props.picked === key ? undefined : key)
}
const groupOf = (key: string) => props.groups.find((g) => g.key === key)
/** Subnets that are no VLAN of the catalog: listed after the VLANs. */
const subnets = computed(() =>
  props.groups.filter((g) => g.vlan === undefined || !vlans.value.some((v) => v.id === g.vlan)),
)
const setNetworkAreas = (on: boolean) => {
  prefs.networkAreas = on
}
</script>

<template>
  <div class="vlans">
    <header class="page-header">
      <div>
        <p class="muted">{{ t('vlans.subtitle') }}</p>
        <label class="areas-toggle">
          <ToggleSwitch
            data-test="toggle-network-areas"
            :model-value="prefs.networkAreas"
            :label="t('mapMenu.networkAreas')"
            @update:model-value="setNetworkAreas"
          />
          <span>{{ t('mapMenu.networkAreas') }}</span>
        </label>
      </div>
      <button
        v-if="editing !== 'new'"
        class="btn primary"
        type="button"
        data-test="new-vlan"
        @click="startNew"
      >
        <Plus :size="16" />{{ t('vlans.new') }}
      </button>
    </header>

    <p v-if="error" class="alert error" role="alert">{{ error }}</p>
    <p v-if="loading" class="muted">{{ t('common.loading') }}</p>

    <VlanForm
      v-if="editing !== undefined"
      :key="String(editing)"
      :vlan="editingVlan"
      :vlans="vlans"
      :router="router"
      :allow-destructive="allowDestructive"
      :wifi="wifiCapable"
      @saved="saved"
      @cancel="editing = undefined"
    />

    <div
      v-if="!loading && !vlans.length && editing === undefined"
      class="card empty"
      data-test="vlans-empty"
    >
      <p>{{ t('vlans.none') }}</p>
      <p class="muted small">{{ t('vlans.noneHint') }}</p>
    </div>

    <ul v-else class="list">
      <li
        v-for="v in sorted"
        :key="v.id"
        class="card vlan"
        :class="{ on: picked === `vlan:${v.id}` }"
        :style="{ '--net': colors.get(`vlan:${v.id}`) ?? vlanHex(v.color) }"
        :data-vlan="v.id"
      >
        <div class="head">
          <button
            type="button"
            class="pick"
            :disabled="!onMap(`vlan:${v.id}`)"
            :title="t('vlans.showOnMap')"
            :data-net="`vlan:${v.id}`"
            :aria-pressed="picked === `vlan:${v.id}`"
            @click="pick(`vlan:${v.id}`)"
          >
            <span class="chip">{{ v.id }}</span>
            <strong>{{ v.name || t('vlans.unnamed') }}</strong>
            <span v-if="v.subnet || groupOf(`vlan:${v.id}`)?.cidr" class="muted mono small">{{
              v.subnet || groupOf(`vlan:${v.id}`)?.cidr
            }}</span>
            <span v-if="groupOf(`vlan:${v.id}`)" class="count">{{
              groupOf(`vlan:${v.id}`)!.members.size
            }}</span>
            <Crosshair v-if="onMap(`vlan:${v.id}`)" :size="14" class="aim" />
          </button>
          <span class="grow" />
          <button
            class="btn ghost icon"
            :class="{ active: portsOpen.has(v.id) }"
            type="button"
            :title="t('vlanPorts.open')"
            :aria-label="t('vlanPorts.open')"
            :aria-expanded="portsOpen.has(v.id)"
            data-test="vlan-ports-toggle"
            @click="togglePorts(v.id)"
          >
            <Cable :size="14" />
          </button>
          <button
            class="btn ghost icon"
            type="button"
            :aria-label="t('vlans.edit')"
            @click="startEdit(v)"
          >
            <Pencil :size="14" />
          </button>
          <button
            v-if="v.id !== 1"
            class="btn ghost icon"
            :class="{ danger: deleting === v.id }"
            type="button"
            :title="deleting === v.id ? t('vlans.confirmDelete') : t('vlans.delete')"
            :aria-label="t('vlans.delete')"
            :disabled="busy"
            data-test="delete-vlan"
            @click="remove(v)"
          >
            <Trash2 :size="14" />
          </button>
        </div>
        <p v-if="summary(v)" class="net-summary small" data-test="net-summary">{{ summary(v) }}</p>
        <p v-if="deleteAgain === v.id" class="small error" data-test="delete-again">
          {{ t('vlans.net.deleteOmini') }}
        </p>
        <p v-else-if="deleting === v.id" class="small warn" data-test="confirm-delete">
          {{
            losing(v)
              ? t('vlans.net.confirmDelete', { n: losing(v) }, losing(v))
              : t('vlans.confirmDelete')
          }}
        </p>
        <VlanPortPanel
          v-if="portsOpen.has(v.id)"
          :vlan="v.id"
          :nodes="nodes"
          :name-of="name"
          :allow-destructive="allowDestructive"
          @applied="load"
        />
        <ul v-else-if="v.devices.length" class="where">
          <li v-for="d in v.devices" :key="d.node_id">
            <button class="link" type="button" @click="open(d.node_id)">
              {{ name(d.node_id) }}
            </button>
            <span class="muted small"> {{ ports(d) || t('vlans.noPorts') }}</span>
          </li>
        </ul>
        <p v-else class="muted small">{{ t('vlans.nowhere') }}</p>
      </li>
    </ul>

    <template v-if="subnets.length">
      <h3 class="section">{{ t('vlans.subnets') }}</h3>
      <ul class="list" data-test="subnets">
        <li
          v-for="g in subnets"
          :key="g.key"
          class="card vlan"
          :class="{ on: picked === g.key }"
          :style="{ '--net': colors.get(g.key) ?? 'var(--text-muted)' }"
        >
          <div class="head">
            <button
              type="button"
              class="pick"
              :title="t('vlans.showOnMap')"
              :data-net="g.key"
              :aria-pressed="picked === g.key"
              @click="pick(g.key)"
            >
              <span class="chip swatch" />
              <strong>{{ g.label || g.cidr || g.name }}</strong>
              <span v-if="g.cidr && g.label" class="muted mono small">{{ g.cidr }}</span>
              <span class="count">{{ g.members.size }}</span>
              <Crosshair :size="14" class="aim" />
            </button>
          </div>
        </li>
      </ul>
    </template>
  </div>
</template>

<style scoped>
.list {
  display: grid;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.vlan {
  padding: 12px 14px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.grow {
  flex: 1;
}
.chip {
  min-width: 34px;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--net);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  text-align: center;
}
.chip.swatch {
  height: 20px;
  border: 2px solid var(--net);
  background: none;
}
.vlan {
  border-left: 3px solid var(--net);
}
.vlan.on {
  border-color: var(--net);
  background: color-mix(in srgb, var(--net) 10%, var(--bg-elevated));
}
.pick {
  display: flex;
  flex: 1;
  align-items: center;
  gap: 10px;
  min-width: 0;
  padding: 0;
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.pick:disabled {
  cursor: default;
}
.pick strong {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.aim {
  flex: none;
  color: var(--text-faint);
}
.pick:hover:not(:disabled) .aim,
.vlan.on .aim {
  color: var(--net);
}
.count {
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--surface-hover);
  color: var(--text-muted);
  font-size: 11.5px;
  font-weight: 600;
}
.section {
  margin: 20px 0 10px;
  font-size: 14px;
}
.areas-toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  font-size: 13px;
  cursor: pointer;
}
.where {
  display: grid;
  gap: 2px;
  margin: 8px 0 0;
  padding: 0;
  list-style: none;
  font-size: 13px;
}
.small {
  font-size: 12.5px;
}
.net-summary {
  margin: 6px 0 0;
  color: var(--text);
}
.error {
  margin: 6px 0 0;
  color: var(--danger);
}
.warn {
  margin: 6px 0 0;
  color: var(--warn);
}
.empty {
  display: grid;
  justify-items: center;
  gap: 6px;
  padding: 32px;
  text-align: center;
}
.empty p {
  margin: 0;
}
</style>
