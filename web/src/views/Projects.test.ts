import { mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Projects from './Projects.vue';

describe('Projects', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => [] })); });
  it('renders the empty state and creates a project through the API', async () => {
    const wrapper = mount(Projects, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } });
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain('Comece pelo seu primeiro projeto');
    await wrapper.get('button.primary').trigger('click');
    const input = document.querySelector('#projects-form input') as HTMLInputElement;
    input.value = 'Website';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    document.querySelector('#projects-form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    expect(fetch).toHaveBeenCalledWith('/api/v1/projects', expect.objectContaining({ method: 'POST' }));
  });
});
