import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import Infrastructure from './Infrastructure.vue';

describe('Infrastructure', () => {
  it('explains when Docker is unavailable', async () => { vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok:true, json:async()=>({available:false,message:'Docker indisponível.',swarm:{active:false}}) })); const wrapper=mount(Infrastructure);await new Promise(resolve=>setTimeout(resolve,0));expect(wrapper.text()).toContain('Docker engine unavailable');expect(wrapper.text()).toContain('Docker indisponível.'); });
});
