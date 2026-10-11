import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import PortVlansEditor from '../map/PortVlansEditor.vue'
import { plugins } from './helpers'

vi.mock('@/lib/api', async (orig) => {
  const mod = await orig<typeof import('@/lib/api')>()
  return {
    ...mod,
    api: {
      ...mod.api,
      vlans: vi.fn<typeof mod.api.vlans>(),
      planVlans: vi.fn<typeof mod.api.planVlans>(),
      applyVlans: vi.fn<typeof mod.api.applyVlans>(),
    },
  }
})

const names: Record<string, string> = {
  'dev:edge': 'sw-edge',
  'dev:core': 'sw-core',
  'dev:fw': 'OPNsense',
}
const mountEditor = (allowDestructive = false) =>
  mount(PortVlansEditor, {
    props: {
      nodeId: 'dev:edge',
      port: { name: 'Port 5', vlans: { untagged: 1 } },
      nameOf: (id: string) => names[id] ?? id,
      allowDestructive,
    },
    global: { plugins: plugins() },
  })

describe('PortVlansEditor', () => {
  beforeEach(() => {
    vi.mocked(api.vlans).mockResolvedValue([
      { id: 1, name: 'default', devices: [] },
      { id: 20, name: 'TV', devices: [] },
    ])
    vi.mocked(api.applyVlans).mockReset()
    vi.mocked(api.applyVlans).mockResolvedValue({ changes: [] })
  })

  it('shows what changes on the way to the router, then applies', async () => {
    vi.mocked(api.planVlans).mockResolvedValue({
      steps: [
        { node_id: 'dev:edge', kind: 'vlan.set', value: { id: 20, name: 'TV' } },
        { node_id: 'dev:edge', kind: 'port.vlans', port: 'Port 5', value: { untagged: 20 } },
        {
          node_id: 'dev:core',
          kind: 'port.vlans',
          port: 'Port 3',
          value: { untagged: 1, tagged: [20] },
        },
      ],
      notes: [{ node_id: 'dev:fw', reason: 'router', vlans: [20] }],
    })
    const w = mountEditor()
    await flushPromises()
    await w.get('[data-test=untagged]').setValue('20')
    await w.get('[data-test=vlan-preview]').trigger('click')
    await flushPromises()
    expect(api.planVlans).toHaveBeenCalledWith('dev:edge', 'Port 5', { untagged: 20 })
    const steps = w.get('[data-test=vlan-plan]').text()
    expect(steps).toContain('sw-edge: create VLAN 20 · TV')
    expect(steps).toContain('sw-core: Port 3 → native 1, tagged 20')
    expect(w.get('[data-test=vlan-notes]').text()).toContain('OPNsense (the router)')
    await w.get('[data-test=vlan-apply]').trigger('click')
    await flushPromises()
    expect(api.applyVlans).toHaveBeenCalledWith('dev:edge', 'Port 5', { untagged: 20 }, false)
    expect(w.emitted('applied')).toHaveLength(1)
  })

  it('blocks a destructive plan unless allowed, then asks twice', async () => {
    vi.mocked(api.planVlans).mockResolvedValue({
      steps: [
        {
          node_id: 'dev:edge',
          kind: 'port.vlans',
          port: 'Port 5',
          value: { untagged: 20 },
          protected: { reason: 'internet' },
        },
      ],
      notes: [],
    })
    let w = mountEditor(false)
    await flushPromises()
    await w.get('[data-test=untagged]').setValue('20')
    await w.get('[data-test=vlan-preview]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test=vlan-apply]').attributes('disabled')).toBeDefined()

    w = mountEditor(true)
    await flushPromises()
    await w.get('[data-test=untagged]').setValue('20')
    await w.get('[data-test=vlan-preview]').trigger('click')
    await flushPromises()
    await w.get('[data-test=vlan-apply]').trigger('click')
    expect(api.applyVlans).not.toHaveBeenCalled()
    await w.get('[data-test=vlan-apply]').trigger('click')
    await flushPromises()
    expect(api.applyVlans).toHaveBeenCalledWith('dev:edge', 'Port 5', { untagged: 20 }, true)
  })
})
