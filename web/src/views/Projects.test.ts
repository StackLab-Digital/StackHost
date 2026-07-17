import { mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Projects from './Projects.vue';

describe('Projects', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => [] })); });
  it('renders the empty state and creates a project through the API', async () => {
    const wrapper = mount(Projects, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } });
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain('No projects yet');
    await wrapper.get('input').setValue('Website');
    await wrapper.get('form').trigger('submit');
    expect(fetch).toHaveBeenCalledWith('/api/v1/projects', expect.objectContaining({ method: 'POST' }));
  });
});
