<script setup lang="ts">
import { Check, Pencil, Plus, Trash2, X } from 'lucide-vue-next'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { api, ApiError } from '@/lib/api'
import { displayName } from '@/lib/names'
import type { NetworkVlan, TopoNode } from '@/lib/types'
import { VLAN_COLORS, vlanHex } from '@/lib/vlans'

/**
 * The network's VLANs: Omini keeps them and applies them to the devices.
 * Create, rename and delete here; put ports in them from a port's card.
 */
const { t } = useI18n()
const router = useRouter()

const vlans = ref<NetworkVlan[]>([])
const nodes = ref(new Map<string, TopoNode>())
const loading = ref(true)
const error = ref('')

async function load() {
  try {
    const [list, topo] = await Promise.all([api.vlans(), api.topology()])
    vlans.value = list
    nodes.value = new Map(topo.topology.nodes.map((n) => [n.id, n]))
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
  void router.push({ name: 'map', query: { node: id } })
}

// One form serves to create and to edit (the id is fixed once created).
const editing = ref<number | 'new'>()
const draft = ref({ id: 0, name: '', subnet: '', color: VLAN_COLORS[0] as string })
const busy = ref(false)
function startNew() {
  const used = new Set(vlans.value.map((v) => v.id))
  let id = 10
  while (used.has(id)) id += 10
  draft.value = {
    id,
    name: '',
    subnet: '',
    color: VLAN_COLORS[vlans.value.length % VLAN_COLORS.length]!,
  }
  editing.value = 'new'
  error.value = ''
}
function startEdit(v: NetworkVlan) {
  draft.value = {
    id: v.id,
    name: v.name,
    subnet: v.subnet ?? '',
    color: v.color || VLAN_COLORS[0]!,
  }
  editing.value = v.id
  error.value = ''
}
async function save() {
  busy.value = true
  error.value = ''
  const d = draft.value
  try {
    if (editing.value === 'new')
      await api.createVlan({ id: Number(d.id), name: d.name, subnet: d.subnet, color: d.color })
    else await api.updateVlan(d.id, { name: d.name, subnet: d.subnet, color: d.color })
    editing.value = undefined
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : t('common.error')
  } finally {
    busy.value = false
  }
}

// Deleting takes a second click; it removes the VLAN from every device first.
const deleting = ref<number>()
async function remove(v: NetworkVlan) {
  if (deleting.value !== v.id) {
    deleting.value = v.id
    return
  }
  busy.value = true
  error.value = ''
  try {
    await api.deleteVlan(v.id)
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : t('common.error')
  } finally {
    busy.value = false
    deleting.value = undefined
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
</script>

<template>
  <div class="page">
    <header class="page-header">
      <div>
        <h1>{{ t('vlans.title') }}</h1>
        <p class="muted">{{ t('vlans.subtitle') }}</p>
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

    <form
      v-if="editing !== undefined"
      class="card editor"
      data-test="vlan-form"
      @submit.prevent="save"
    >
      <label class="field id">
        <span>{{ t('vlans.id') }}</span>
        <input
          v-model.number="draft.id"
          class="input mono"
          type="number"
          min="1"
          max="4094"
          required
          :disabled="editing !== 'new'"
        />
      </label>
      <label class="field">
        <span>{{ t('vlans.name') }}</span>
        <input
          v-model="draft.name"
          class="input"
          maxlength="32"
          :placeholder="t('vlans.namePlaceholder')"
        />
      </label>
      <label class="field">
        <span>{{ t('vlans.subnet') }}</span>
        <input v-model="draft.subnet" class="input mono" placeholder="192.168.20.0/24" />
      </label>
      <div class="field">
        <span>{{ t('vlans.color') }}</span>
        <div class="colors">
          <button
            v-for="c in VLAN_COLORS"
            :key="c"
            type="button"
            class="swatch"
            :class="{ on: draft.color === c }"
            :style="{ background: vlanHex(c) }"
            :aria-label="c"
            @click="draft.color = c"
          />
        </div>
      </div>
      <div class="editor-actions">
        <button class="btn ghost" type="button" @click="editing = undefined">
          <X :size="15" />{{ t('common.cancel') }}
        </button>
        <button class="btn primary" type="submit" :disabled="busy">
          <Check :size="15" />{{ t('common.save') }}
        </button>
      </div>
      <p v-if="editing !== 'new'" class="muted small hint">{{ t('vlans.renameHint') }}</p>
    </form>

    <div
      v-if="!loading && !vlans.length && editing === undefined"
      class="card empty"
      data-test="vlans-empty"
    >
      <p>{{ t('vlans.none') }}</p>
      <p class="muted small">{{ t('vlans.noneHint') }}</p>
    </div>

    <ul v-else class="list">
      <li v-for="v in sorted" :key="v.id" class="card vlan" :data-vlan="v.id">
        <div class="head">
          <span class="chip" :style="{ background: vlanHex(v.color) }">{{ v.id }}</span>
          <strong>{{ v.name || t('vlans.unnamed') }}</strong>
          <span v-if="v.subnet" class="muted mono small">{{ v.subnet }}</span>
          <span class="grow" />
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
        <p v-if="deleting === v.id" class="small warn">{{ t('vlans.confirmDelete') }}</p>
        <ul v-if="v.devices.length" class="where">
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
  </div>
</template>

<style scoped>
.editor {
  display: grid;
  grid-template-columns: 110px 1fr 1fr;
  gap: 12px;
  align-items: end;
  margin-bottom: 16px;
  padding: 14px;
}
.field {
  display: grid;
  gap: 4px;
  font-size: 12.5px;
  color: var(--text-muted);
}
.colors {
  display: flex;
  gap: 6px;
}
.swatch {
  width: 22px;
  height: 22px;
  border: 2px solid transparent;
  border-radius: 50%;
  cursor: pointer;
}
.swatch.on {
  border-color: var(--text);
}
.editor-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  grid-column: 2 / -1;
}
.hint {
  grid-column: 1 / -1;
  margin: 0;
}
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
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  text-align: center;
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
@media (max-width: 760px) {
  .editor {
    grid-template-columns: 1fr;
  }
  .editor-actions {
    grid-column: auto;
  }
}
</style>
