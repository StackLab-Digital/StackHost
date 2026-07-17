import { defineStore } from 'pinia';

export const useAuthStore = defineStore('auth', {
  state: () => ({ user: null as null | { id: number; name: string; email: string; role: string }, loaded: false }),
  actions: {
    async load() { const response = await fetch('/api/v1/me'); this.user = response.ok ? await response.json() : null; this.loaded = true; return this.user; },
    clear() { this.user = null; this.loaded = true; }
  }
});
