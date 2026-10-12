<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api, ApiError } from '@/lib/api'
import type { Interface, NetworkVlan, PortVlans, VlanPlan } from '@/lib/types'
import { describeVlans } from '@/lib/vlans'

import VlanPlanSteps from './VlanPlanSteps.vue'

/**
 * A port's VLANs: access (one VLAN) or trunk (tagged VLANs, with a native one
 * or none), from the network's VLANs. Before applying, it shows what changes
 * on every device on the way to the router.
 */
const props = defineProps<{
  nodeId: string
  port: Interface
  /** A node's name on the map. */
  nameOf: (id: string) => string
  /** Destructive changes are allowed (Settings). */
  allowDestructive?: boolean
}>()
const emit = defineEmits<{ close: []; applied: [] }>()
const { t } = useI18n()

const catalog = ref<NetworkVlan[]>([])
onMounted(async () => {
  try {
    catalog.value = await api.vlans()
  } catch {
    // shown as no VLANs to choose
  }
})

const now = props.port.vlans
const mode = ref<'access' | 'trunk'>(now?.tagged?.length ? 'trunk' : 'access')
const untagged = ref<number | undefined>(now?.untagged)
const tagged = ref<number[]>([...(now?.tagged ?? [])])

const want = computed<PortVlans>(() =>
  mode.value === 'access'
    ? { untagged: untagged.value }
    : { untagged: untagged.value, tagged: tagged.value.filter((v) => v !== untagged.value) },
)
const valid = computed(() =>
  mode.value === 'access' ? untagged.value !== undefined : want.value.tagged!.length > 0,
)
const label = (id: number) => {
  const v = catalog.value.find((x) => x.id === id)
  return v?.name ? `${id} · ${v.name}` : String(id)
}
const describe = (v: PortVlans) => describeVlans(v, t)

const plan = ref<VlanPlan>()
const busy = ref(false)
const error = ref('')
const destructive = computed(() => plan.value?.steps.some((s) => s.protected) ?? false)
const again = ref(false)

async function preview() {
  busy.value = true
  error.value = ''
  try {
    plan.value = await api.planVlans(props.nodeId, props.port.name, want.value)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : t('common.error')
  } finally {
    busy.value = false
  }
}
async function apply() {
  if (destructive.value && !again.value) {
    again.value = true
    return
  }
  busy.value = true
  error.value = ''
  try {
    await api.applyVlans(props.nodeId, props.port.name, want.value, destructive.value)
    emit('applied')
    emit('close')
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : t('common.error')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="vlan-editor" data-test="vlan-editor">
    <template v-if="!plan">
      <div class="modes" role="radiogroup">
        <label
          ><input v-model="mode" type="radio" value="access" />
          {{ t('portVlans.modeAccess') }}</label
        >
        <label
          ><input v-model="mode" type="radio" value="trunk" /> {{ t('portVlans.modeTrunk') }}</label
        >
      </div>
      <label class="field">
        <span>{{ mode === 'access' ? t('portVlans.vlan') : t('portVlans.native') }}</span>
        <select v-model="untagged" class="select" data-test="untagged">
          <option v-if="mode === 'trunk'" :value="undefined">{{ t('portVlans.noNative') }}</option>
          <option v-for="v in catalog" :key="v.id" :value="v.id">{{ label(v.id) }}</option>
        </select>
      </label>
      <fieldset v-if="mode === 'trunk'" class="field tagged">
        <legend>{{ t('portVlans.tagged') }}</legend>
        <label v-for="v in catalog.filter((x) => x.id !== untagged)" :key="v.id">
          <input v-model="tagged" type="checkbox" :value="v.id" :data-test="`tag-${v.id}`" />
          {{ label(v.id) }}
        </label>
      </fieldset>
      <p v-if="!catalog.length" class="muted small">{{ t('portVlans.noCatalog') }}</p>
      <div class="actions">
        <button class="btn small ghost" type="button" @click="emit('close')">
          {{ t('changes.cancel') }}
        </button>
        <button
          class="btn small primary"
          type="button"
          data-test="vlan-preview"
          :disabled="!valid || busy"
          @click="preview"
        >
          {{ t('portVlans.preview') }}
        </button>
      </div>
    </template>

    <template v-else>
      <p class="small">{{ t('portVlans.planFor', { port: port.name, vlans: describe(want) }) }}</p>
      <VlanPlanSteps :plan="plan" :name-of="nameOf" />
      <p v-if="destructive && !allowDestructive" class="blocked">{{ t('portVlans.blocked') }}</p>
      <p v-else-if="again" class="confirm-again">{{ t('portVlans.askAgain') }}</p>
      <div class="actions">
        <button class="btn small ghost" type="button" :disabled="busy" @click="plan = undefined">
          {{ t('portVlans.back') }}
        </button>
        <button
          class="btn small"
          :class="destructive ? 'danger' : 'primary'"
          type="button"
          data-test="vlan-apply"
          :disabled="busy || (destructive && !allowDestructive)"
          @click="apply"
        >
          {{
            busy ? t('changes.applying') : again ? t('portVlans.applyAnyway') : t('portVlans.apply')
          }}
        </button>
      </div>
    </template>
    <p v-if="error" class="change-error" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.vlan-editor {
  display: grid;
  gap: 10px;
  margin-top: 12px;
  padding: 10px 12px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  background: var(--surface);
}
.modes {
  display: flex;
  gap: 16px;
}
.field {
  display: grid;
  gap: 4px;
  margin: 0;
  padding: 0;
  border: 0;
  font-size: 12.5px;
}
.field > span,
.field legend {
  color: var(--text-muted);
}
.tagged label {
  display: block;
}
.actions {
  display: flex;
  gap: 6px;
  justify-content: flex-end;
}
.blocked,
.confirm-again,
.change-error {
  margin: 0;
  font-size: 12px;
}
.blocked,
.change-error {
  color: var(--danger);
}
.confirm-again {
  color: var(--warn);
}
.small {
  margin: 0;
  font-size: 12.5px;
}
</style>
