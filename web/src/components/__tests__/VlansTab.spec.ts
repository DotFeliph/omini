import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import VlansTab from '../map/VlansTab.vue'
import { plugins } from './helpers'

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

function mountView(props = {}) {
  return mount(VlansTab, {
    props: { groups: [], colors: new Map<string, string>(), ...props },
    global: { plugins: plugins() },
  })
}

describe('VlansTab', () => {
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
    expect(api.deleteVlan).toHaveBeenCalledWith(20, false)
  })

  it('highlights a VLAN or a subnet on the map with a click, and lists the subnets', async () => {
    const groups = [
      { key: 'vlan:1', name: 'VLAN 1', vlan: 1, members: new Set(['dev:sw']) },
      {
        key: 'subnet:10.0.5.0/24',
        name: 'LAB · 10.0.5.0/24',
        label: 'LAB',
        cidr: '10.0.5.0/24',
        members: new Set(['a', 'b']),
      },
    ]
    const w = mountView({ groups })
    await flushPromises()
    await w.get('[data-net="vlan:1"]').trigger('click')
    expect(w.emitted('pick')).toEqual([['vlan:1']])
    expect(w.get('[data-test=subnets]').text()).toContain('LAB')
    expect(w.get('[data-test=subnets]').text()).toContain('2')
    await w.setProps({ picked: 'subnet:10.0.5.0/24' })
    await w.get('[data-net="subnet:10.0.5.0/24"]').trigger('click')
    expect(w.emitted('pick')![1]).toEqual([undefined])
  })
})
