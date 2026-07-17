import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import Dashboard from './Dashboard.vue'

describe('Dashboard', () => {
  it('renderiza o estado vazio e o diagnóstico do Docker', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ projects: 0, applications: 0, activity: [], infrastructure: { available: false, message: 'Docker indisponível.' } }) }))
    vi.stubGlobal('EventSource', class { close() {} })
    const wrapper = mount(Dashboard, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.text()).toContain('Novo projeto')
    expect(wrapper.text()).toContain('Docker indisponível.')
  })
})
