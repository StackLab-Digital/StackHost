<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import BaseModal from "../ui/BaseModal.vue";
import Checkbox from "../ui/Checkbox.vue";
import { api, RequestError } from "../../composables/useApi";
import { useToast } from "../../composables/useToast";

type Domain = {
  id: number;
  hostname: string;
  service_name: string;
  target_port: number;
  primary_domain: boolean;
  https_enabled: boolean;
  redirect_https: boolean;
  enabled: boolean;
  status: string;
  certificate_status: string;
  last_error?: string;
};

const props = defineProps<{ applicationId: number; services: string[] }>();
const toast = useToast();
const domains = ref<Domain[]>([]);
const loading = ref(true);
const saving = ref(false);
const hostname = ref("");
const serviceName = ref("");
const targetPort = ref(80);
const httpsEnabled = ref(true);
const redirectHttps = ref(true);
const addOpen = ref(false);

async function load() {
  loading.value = true;
  try {
    const result = await api<Domain[]>(`/api/v1/applications/${props.applicationId}/domains`);
    domains.value = Array.isArray(result) ? result : [];
    if (!serviceName.value && props.services.length) serviceName.value = props.services[0];
  } catch (error) {
    toast.error(error instanceof Error ? error.message : "Não foi possível carregar os domínios.");
  } finally {
    loading.value = false;
  }
}
async function addDomain() {
  saving.value = true;
  try {
    await api(`/api/v1/applications/${props.applicationId}/domains`, {
      method: "POST",
      body: JSON.stringify({ hostname: hostname.value, service_name: serviceName.value, target_port: targetPort.value, https_enabled: httpsEnabled.value, redirect_https: redirectHttps.value }),
    });
    hostname.value = "";
    addOpen.value = false;
    toast.success("Domínio cadastrado.");
    await load();
  } catch (error) {
    toast.error(error instanceof RequestError ? error.message : "Não foi possível cadastrar o domínio.");
  } finally {
    saving.value = false;
  }
}
function openAddDomain() {
  if (!serviceName.value && props.services.length) serviceName.value = props.services[0];
  addOpen.value = true;
}
async function checkDomain(domain: Domain) {
  try {
    const updated = await api<Domain>(`/api/v1/applications/${props.applicationId}/domains/${domain.id}/check`, { method: "POST" });
    domains.value = domains.value.map((item) => (item.id === updated.id ? updated : item));
    toast.info(updated.last_error || "Domínio verificado.");
  } catch (error) {
    toast.error(error instanceof Error ? error.message : "Não foi possível verificar o domínio.");
  }
}
async function removeDomain(domain: Domain) {
  if (!window.confirm(`Remover ${domain.hostname}?`)) return;
  try {
    await api(`/api/v1/applications/${props.applicationId}/domains/${domain.id}`, { method: "DELETE" });
    domains.value = domains.value.filter((item) => item.id !== domain.id);
    toast.success("Domínio removido.");
  } catch (error) {
    toast.error(error instanceof Error ? error.message : "Não foi possível remover o domínio.");
  }
}
watch(() => props.applicationId, load);
onMounted(load);
</script>

<template>
  <section class="domains-panel">
    <div class="section-head">
      <div>
        <p class="kicker">DOMÍNIOS</p>
        <h2>Endereços públicos</h2>
        <p class="muted">O StackHost controla o proxy, HTTPS e a rota até o serviço real.</p>
      </div>
      <button class="primary" type="button" @click="openAddDomain">Adicionar domínio</button>
    </div>
    <div v-if="loading" class="empty"><p>Carregando domínios…</p></div>
    <div v-else-if="!domains.length" class="empty"><p>Nenhum domínio cadastrado ainda.</p></div>
    <ul v-else class="domain-list">
      <li v-for="domain in domains" :key="domain.id">
        <div><strong>{{ domain.hostname }}</strong><span class="muted">{{ domain.service_name }}:{{ domain.target_port }}</span></div>
        <div class="domain-actions"><span class="status-badge" :class="domain.status">{{ domain.status }}</span><button class="ghost" type="button" @click="checkDomain(domain)">Verificar</button><button class="ghost danger" type="button" @click="removeDomain(domain)">Remover</button></div>
        <p v-if="domain.last_error" class="domain-error">{{ domain.last_error }}</p>
      </li>
    </ul>
    <BaseModal :open="addOpen" title="Adicionar domínio" description="Configure o endereço público e o serviço de destino." @close="addOpen = false">
      <form id="add-domain-form" class="domain-form" @submit.prevent="addDomain">
        <label>Domínio<input v-model="hostname" required autofocus placeholder="app.exemplo.com" /></label>
        <label v-if="props.services.length">Serviço<select v-model="serviceName" required><option v-for="service in props.services" :key="service" :value="service">{{ service }}</option></select></label>
        <label v-else>Serviço<input v-model="serviceName" required placeholder="web" /></label>
        <label>Porta<input v-model.number="targetPort" type="number" min="1" max="65535" required /></label>
        <Checkbox v-model="httpsEnabled" label="HTTPS automático" />
        <Checkbox v-model="redirectHttps" label="Redirecionar HTTP" />
      </form>
      <template #footer><button class="secondary" type="button" :disabled="saving" @click="addOpen = false">Cancelar</button><button class="primary" form="add-domain-form" type="submit" :disabled="saving">{{ saving ? "Salvando…" : "Adicionar domínio" }}</button></template>
    </BaseModal>
  </section>
</template>

<style scoped>
.domains-panel { display: grid; gap: 22px; }
.domain-form { display: grid; grid-template-columns: minmax(220px, 1.5fr) minmax(140px, 1fr) 110px; gap: 14px; align-items: end; }
.domain-form label { display: grid; gap: 7px; color: #b9bec9; font-size: 12px; }
.domain-form input, .domain-form select { min-width: 0; border: 1px solid #343842; border-radius: 7px; padding: 10px 11px; background: #17191e; color: #e7e9ee; }
.domain-list { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
.domain-list li { display: grid; gap: 10px; padding: 16px; border: 1px solid #2a2d34; border-radius: 10px; background: #17191e; }
.domain-list li > div:first-child { display: flex; gap: 14px; align-items: center; }
.domain-actions { display: flex; align-items: center; gap: 8px; }
.domain-actions .danger { color: #efb3b3; }
.domain-error { margin: 0; color: #efb3b3; font-size: 12px; }
@media (max-width: 700px) { .domain-form { grid-template-columns: 1fr; } .domain-actions { flex-wrap: wrap; } }
</style>
