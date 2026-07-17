import { computed, reactive } from 'vue'

export type Toast = { id: number; kind: 'success' | 'error' | 'warning' | 'info'; message: string }
const state = reactive<{ items: Toast[] }>({ items: [] })
let nextId = 1
export function useToast() {
  const remove = (id: number) => { state.items = state.items.filter(item => item.id !== id) }
  const show = (kind: Toast['kind'], message: string) => { const id = nextId++; state.items.push({ id, kind, message }); window.setTimeout(() => remove(id), 4500) }
  return { toasts: computed(() => state.items), show, remove, success: (message: string) => show('success', message), error: (message: string) => show('error', message), info: (message: string) => show('info', message), warning: (message: string) => show('warning', message) }
}
