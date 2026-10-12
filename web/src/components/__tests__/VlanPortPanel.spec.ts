import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import type { TopoNode } from '@/lib/types'

import VlanPortPanel from '../map/VlanPortPanel.vue'
import { plugins } from './helpers'

vi.mock('@/lib/api', async (orig) => {
  const mod = await orig<typeof import('@/lib/api')>()
  return {
    ...mod,
    api: {
      ...mod.api,
      planVlans: vi.fn<typeof mod.api.planVlans>(),
      applyVlans: vi.fn<typeof mod.api.applyVlans>(),
    },
  }
})

const sw: TopoNode = {
  id: 'dev:sw',
  kind: 'device',
  label: 'sw',
  online: true,
  changes: ['port.vlans', 'vlan.set'],
  protected: { 'Port 9': { reason: 'internet' } },
  device: {
    key: 'sw',
    name: 'sw',
    interfaces: [
      { name: 'Port 10', vlans: { untagged: 1 } },
      { name: 'Port 2', vlans: { untagged: 30 } },
      { name: 'Port 9', vlans: { untagged: 1, tagged: [20, 30] } },
      { name: 'Port 1', vlans: { untagged: 1 } },
    ],
  },
}
const ap: TopoNode = { id: 'dev:ap', kind: 'device', label: 'ap', online: true }
const panel = () =>
  mount(VlanPortPanel, {
    props: {
      vlan: 30,
      nodes: new Map([
        [sw.id, sw],
        [ap.id, ap],
      ]),
      nameOf: (id: string) => id,
      allowDestructive: false,
    },
    global: { plugins: plugins() },
  })

describe('VlanPortPanel', () => {
  beforeEach(() => {
    vi.mocked(api.planVlans).mockResolvedValue({ steps: [], notes: [] })
    vi.mocked(api.applyVlans).mockResolvedValue({ changes: [] })
  })

  it("shows each switch's ports in order, in, tagged or out of the VLAN", () => {
    const w = panel()
    expect(w.findAll('[data-switch]')).toHaveLength(1) // the AP changes no VLANs
    const ports = w.findAll('[data-port]')
    expect(ports.map((p) => p.attributes('data-port'))).toEqual([
      'Port 1',
      'Port 2',
      'Port 9',
      'Port 10',
    ])
    expect(ports.map((p) => p.classes().find((c) => ['in', 'tagged', 'out'].includes(c)))).toEqual([
      'out',
      'in',
      'tagged',
      'out',
    ])
  })

  it('moves a port out → in → tagged → out, showing the plan first', async () => {
    const w = panel()
    await w.get('[data-port="Port 1"]').trigger('click')
    await flushPromises()
    expect(api.planVlans).toHaveBeenLastCalledWith('dev:sw', 'Port 1', { untagged: 30 })
    await w.get('[data-test=port-apply]').trigger('click')
    await flushPromises()
    expect(api.applyVlans).toHaveBeenCalledWith('dev:sw', 'Port 1', { untagged: 30 }, false)
    expect(w.emitted('applied')).toHaveLength(1)

    await w.get('[data-port="Port 2"]').trigger('click') // in → tagged: VLAN 1 native
    expect(api.planVlans).toHaveBeenLastCalledWith('dev:sw', 'Port 2', {
      untagged: 1,
      tagged: [30],
    })
    await w.get('[data-port="Port 9"]').trigger('click') // tagged → out: keeps the rest
    expect(api.planVlans).toHaveBeenLastCalledWith('dev:sw', 'Port 9', {
      untagged: 1,
      tagged: [20],
    })
  })

  it('asks for the destructive step to be allowed', async () => {
    vi.mocked(api.planVlans).mockResolvedValue({
      steps: [
        {
          node_id: 'dev:sw',
          kind: 'port.vlans',
          port: 'Port 9',
          value: {},
          protected: { reason: 'internet' },
        },
      ],
      notes: [],
    })
    const w = panel()
    await w.get('[data-port="Port 9"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test=port-apply]').attributes('disabled')).toBeDefined()
  })
})
