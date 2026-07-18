<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { api } from "../../composables/useApi";
import { useToast } from "../../composables/useToast";
import Checkbox from "../ui/Checkbox.vue";

const props = defineProps<{ applicationId: number }>();
const toast = useToast();
const services = ref<string[]>([]);
const service = ref("");
const tail = ref(100);
const logs = ref("");
const loading = ref(false);
const following = ref(false);
const query = ref("");
const autoScroll = ref(true);
const connected = ref(false);
const viewer = ref<HTMLElement>();
let events: EventSource | undefined;

async function loadServices() {
  try {
    const result = await api<Record<string, unknown>>(`/api/v1/applications/${props.applicationId}/logs/services`);
    services.value = Object.keys(result || {});
    if (!service.value) service.value = services.value[0] || "";
  } catch (error) { toast.error(error instanceof Error ? error.message : "Não foi possível carregar os serviços."); }
}
async function loadLogs() {
  if (!service.value) return;
  loading.value = true;
	try { const result = await api<{ logs: string }>(`/api/v1/applications/${props.applicationId}/logs?service=${encodeURIComponent(service.value)}&tail=${tail.value}`); logs.value = result.logs || ""; await nextTick(); if (autoScroll.value && viewer.value) viewer.value.scrollTop = viewer.value.scrollHeight; }
  catch (error) { toast.error(error instanceof Error ? error.message : "Não foi possível carregar os logs."); }
  finally { loading.value = false; }
}
function stopFollowing() { following.value = false; connected.value = false; events?.close(); events = undefined; }
function toggleFollowing() {
  if (following.value) { stopFollowing(); return; }
  if (!service.value) return;
  events = new EventSource(`/api/v1/applications/${props.applicationId}/logs/stream?service=${encodeURIComponent(service.value)}`);
  events.onmessage = (event) => {
    try { const payload = JSON.parse(event.data) as { line?: string }; if (payload.line !== undefined) { logs.value += `${logs.value ? "\n" : ""}${payload.line}`; void nextTick(() => { if (autoScroll.value && viewer.value) viewer.value.scrollTop = viewer.value.scrollHeight; }); } }
    catch { /* ignore malformed frames */ }
  };
  events.onopen = () => { connected.value = true; };
  events.onerror = () => { connected.value = false; };
  following.value = true;
}
async function copyLogs() { await navigator.clipboard?.writeText(logs.value); toast.info("Logs copiados."); }
function downloadLogs() { const link = document.createElement("a"); link.href = URL.createObjectURL(new Blob([logs.value], { type: "text/plain" })); link.download = `${service.value || "logs"}.txt`; link.click(); URL.revokeObjectURL(link.href); }
watch(() => props.applicationId, async () => { stopFollowing(); logs.value = ""; await loadServices(); await loadLogs(); });
watch(service, () => void loadLogs());
onMounted(async () => { await loadServices(); await loadLogs(); });
onUnmounted(stopFollowing);
</script>

<template>
  <section class="logs-panel">
    <div class="section-head"><div><p class="kicker">LOGS</p><h2>Saída operacional</h2><p class="muted">{{ connected ? "Conectado em tempo real" : "Histórico do serviço" }}</p></div><div class="log-actions"><button class="ghost" type="button" :disabled="!logs" @click="copyLogs">Copiar</button><button class="ghost" type="button" :disabled="!logs" @click="downloadLogs">Baixar</button><button class="ghost" type="button" :disabled="!logs" @click="logs = ''">Limpar</button><button class="secondary" type="button" :class="{ active: following }" @click="toggleFollowing">{{ following ? "Pausar" : "Acompanhar" }}</button></div></div>
    <div class="logs-toolbar"><label>Serviço<select v-model="service"><option v-for="item in services" :key="item" :value="item">{{ item }}</option></select></label><label>Linhas<select v-model.number="tail"><option :value="100">100</option><option :value="500">500</option><option :value="1000">1000</option></select></label><label>Buscar<input v-model="query" type="search" placeholder="Filtrar linhas" /></label><Checkbox v-model="autoScroll" label="Auto-scroll" /><button class="ghost" type="button" @click="loadLogs">Atualizar</button></div>
    <pre ref="viewer" class="logs-viewer" :aria-busy="loading">{{ (logs.split("\n").filter((line) => !query || line.toLowerCase().includes(query.toLowerCase())).join("\n")) || "Nenhum log disponível para este serviço." }}</pre>
  </section>
</template>

<style scoped>
.logs-panel { display: grid; gap: 18px; }
.log-actions, .logs-toolbar { display: flex; flex-wrap: wrap; align-items: end; gap: 8px; }
.logs-toolbar { padding: 12px; border: 1px solid #2a2d34; border-radius: 8px; background: #17191e; }
.logs-toolbar label { display: grid; gap: 5px; color: #858b99; font-size: 12px; }
.logs-toolbar select { border: 1px solid #343842; border-radius: 6px; padding: 8px; background: #111317; color: #e7e9ee; }
.logs-viewer { min-height: 360px; max-height: 60vh; overflow: auto; margin: 0; padding: 16px; border: 1px solid #2a2d34; border-radius: 8px; background: #111317; color: #d7dbe3; font: 12px/1.6 "Space Mono", monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
