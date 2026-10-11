import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { api } from '@/lib/api'

import { plugins } from '../../components/__tests__/helpers'
import VlansView from '../VlansView.vue'

vi.mock('@/lib/api', async (orig) => {
  const mod = await orig<typeof import('@/lib/api')>()
  return {
    ...mod,
    api: {
      ...mod.api,
      vlans: vi.fn<typeof mod.api.vlans>(),
      createVlan: vi.fn<typeof mod.api.createVlan>(),
      updateVlan: vi.fn<typeof mod.api.updateVlan>(),
      deleteVlan: vi.fn<typeof mod.api.deleteVlan>(),
      topology: vi.fn<typeof mod.api.topology>(),
    },
  }
})

function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'map', component: { template: '<div />' } },
      { path: '/vlans', component: VlansView },
    ],
  })
  return mount(VlansView, { global: { plugins: [...plugins(), router] } })
}

describe('VlansView', () => {
  beforeEach(() => {
    vi.mocked(api.vlans).mockResolvedValue([
      {
        id: 1,
        name: 'default',
        devices: [{ node_id: 'dev:sw', untagged: ['Port 1', 'Port 2'], tagged: [] }],
      },
      { id: 20, name: 'Guests', color: 'green', subnet: '192.168.20.0/24', devices: [] },
    ])
    vi.mocked(api.topology).mockResolvedValue({
      topology: {
        nodes: [{ id: 'dev:sw', kind: 'device', label: 'HC-SWTGW218AS', online: true }],
        edges: [],
      },
      statuses: [],
      generated_at: '',
      layout: {},
    })
    vi.mocked(api.createVlan).mockReset()
    vi.mocked(api.deleteVlan).mockReset()
  })

  it('lists the VLANs and where they are', async () => {
    const w = mountView()
    await flushPromises()
    const one = w.get('[data-vlan="1"]').text()
    expect(one).toContain('HC-SWTGW218AS')
    expect(one).toContain('untagged: Port 1, Port 2')
    expect(w.get('[data-vlan="20"]').text()).toContain('192.168.20.0/24')
    expect(w.find('[data-vlan="1"] [data-test=delete-vlan]').exists()).toBe(false) // VLAN 1 stays
  })

  it('creates a VLAN with the next free id', async () => {
    vi.mocked(api.createVlan).mockResolvedValue({
      vlan: { id: 10, name: 'Office', devices: [] },
      changes: [],
    })
    const w = mountView()
    await flushPromises()
    await w.get('[data-test=new-vlan]').trigger('click')
    const inputs = w.findAll('[data-test=vlan-form] input')
    expect((inputs[0]!.element as HTMLInputElement).value).toBe('10')
    await inputs[1]!.setValue('Office')
    await w.get('[data-test=vlan-form]').trigger('submit')
    await flushPromises()
    expect(api.createVlan).toHaveBeenCalledWith(expect.objectContaining({ id: 10, name: 'Office' }))
  })

  it('deletes with a second click', async () => {
    vi.mocked(api.deleteVlan).mockResolvedValue({ changes: [] })
    const w = mountView()
    await flushPromises()
    const del = w.get('[data-vlan="20"] [data-test=delete-vlan]')
    await del.trigger('click')
    expect(api.deleteVlan).not.toHaveBeenCalled()
    await del.trigger('click')
    await flushPromises()
    expect(api.deleteVlan).toHaveBeenCalledWith(20)
  })
})
