<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { RouterLink } from "vue-router";
import BaseModal from "../components/ui/BaseModal.vue";
import { api, RequestError } from "../composables/useApi";
import { useToast } from "../composables/useToast";
import type { Infrastructure, Project } from "../types";
const toast = useToast();
const loading = ref(true);
const error = ref("");
const data = ref<any>({ projects: 0, applications: 0, infrastructure: {} });
const showProject = ref(false);
const saving = ref(false);
const fields = ref({ name: "", description: "" });
const fieldErrors = ref<Record<string, string>>({});
let source: EventSource | undefined;
let pollTimer: number | undefined;
let refreshing = false;
const refreshEvents = new Set([
  "project.created",
  "project.updated",
  "application.created",
  "application.updated",
  "application.deleted",
  "application.source_configured",
  "application.source_validated",
  "application.source_invalid",
  "application.source_changed",
  "swarm.initialized",
]);
const nowGreeting = computed(() => {
  const hour = new Date().getHours();
  return hour < 12 ? "Bom dia" : hour < 18 ? "Boa tarde" : "Boa noite";
});
const infra = computed(() => ((data.value && (data.value.docker || data.value.infrastructure)) || {}) as Infrastructure);
const overallMessage = computed(() => {
  if (!infra.value.available) return "Docker indisponível";
  const count = (data.value.alerts || []).length;
  return count ? `${count} ${count === 1 ? "item precisa" : "itens precisam"} de atenção` : "Tudo funcionando";
});
const userName = ref("Administrador");
async function load(silent = false) {
  if (refreshing) return;
  refreshing = true;
  if (!silent) {
    loading.value = true;
    error.value = "";
  }
  try {
    data.value = await api("/api/v1/dashboard");
    if (silent) {
      error.value = "";
    } else {
      try {
        const me = await api<{ name: string }>("/api/v1/me");
        if (me.name) userName.value = me.name.split(" ")[0];
      } catch {
        /* dashboard remains useful if the profile request is unavailable */
      }
    }
  } catch (err) {
    if (!silent) {
      error.value =
        err instanceof Error
          ? err.message
          : "Não foi possível carregar o painel.";
    }
  } finally {
    if (!silent) loading.value = false;
    refreshing = false;
  }
}
function stopPolling() {
  if (pollTimer === undefined) return;
  window.clearInterval(pollTimer);
  pollTimer = undefined;
}
function startPolling() {
  if (pollTimer !== undefined) return;
  void load(true);
  pollTimer = window.setInterval(() => void load(true), 30_000);
}
function handleEvent(event: MessageEvent<string>) {
  let message: { event?: string; data?: Infrastructure };
  try {
    message = JSON.parse(event.data);
  } catch {
    return;
  }
  if (message.event === "infrastructure.updated" && message.data) {
    data.value = { ...data.value, infrastructure: message.data };
  } else if (message.event && refreshEvents.has(message.event)) {
    void load(true);
  }
}
async function createProject() {
  saving.value = true;
  fieldErrors.value = {};
  try {
    await api<Project>("/api/v1/projects", {
      method: "POST",
      body: JSON.stringify(fields.value),
    });
    showProject.value = false;
    fields.value = { name: "", description: "" };
    toast.success("Projeto criado com sucesso.");
    await load();
  } catch (err) {
    if (err instanceof RequestError) {
      fieldErrors.value = err.fields || {};
      toast.error(err.message);
    }
  } finally {
    saving.value = false;
  }
}
onMounted(async () => {
  await load();
  try {
    source = new EventSource("/api/v1/events");
    source.onmessage = handleEvent;
    source.onopen = stopPolling;
    source.onerror = startPolling;
  } catch {
    startPolling();
  }
});
onUnmounted(() => {
  source?.close();
  stopPolling();
});
</script>
<template>
  <div class="hero">
    <div>
      <p class="kicker">VISÃO GERAL</p>
      <h1>{{ nowGreeting }}, {{ userName }}</h1>
      <p class="muted">Aqui está o estado do seu ambiente.</p>
    </div>
    <button class="primary" type="button" @click="showProject = true">
      + Novo projeto
    </button>
  </div>
  <div v-if="loading" class="grid">
    <article v-for="n in 3" :key="n" class="skeleton-card">
      <span class="skeleton" />
    </article>
  </div>
  <div v-else-if="error" class="empty">
    <h2>Não foi possível carregar o painel</h2>
    <p>{{ error }}</p>
    <button class="secondary" type="button" @click="load()">
      Tentar novamente
    </button>
  </div>
  <template v-else>
    <section class="status">
      <span class="dot" :class="{ offline: !infra.available }" />
      <div>
        <strong>{{
          infra.available
            ? infra.swarm?.active
              ? "Ambiente preparado"
              : "Docker conectado"
            : "Docker indisponível"
        }}</strong>
        <p>
          {{
            infra.available
              ? `Docker ${infra.engine_version || "conectado"} · ${infra.running || 0} de ${infra.containers || 0} containers em execução`
              : infra.message ||
                "Verifique se o Docker Desktop está em execução."
          }}
        </p>
      </div>
      <span class="pill">{{ overallMessage }}</span>
    </section>
    <section v-if="data.applications" class="grid metrics operational-cards">
      <RouterLink class="panel operational-card" to="/applications"><span class="label">APLICAÇÕES</span><strong>{{ data.applications.total }}</strong><span>{{ data.applications.running || 0 }} em execução · {{ data.applications.degraded || 0 }} degradadas</span></RouterLink>
      <RouterLink class="panel operational-card" to="/applications"><span class="label">DEPLOYS</span><strong>{{ data.deployments?.running || 0 }}</strong><span>{{ data.deployments?.failed_last_24h || 0 }} falhos em 24h</span></RouterLink>
      <RouterLink class="panel operational-card" to="/applications"><span class="label">DOMÍNIOS</span><strong>{{ data.domains?.active || 0 }}</strong><span>{{ data.domains?.pending || 0 }} pendentes · {{ data.domains?.errors || 0 }} com erro</span></RouterLink>
      <RouterLink class="panel operational-card" to="/settings"><span class="label">BACKUPS</span><strong>{{ data.backups?.last_status || "—" }}</strong><span>{{ data.backups?.last_created_at ? new Date(data.backups.last_created_at).toLocaleString("pt-BR") : "Nenhum backup registrado" }}</span></RouterLink>
    </section>
    <section v-if="data.alerts?.length" class="panel panel-section operational-alerts">
      <div class="section-head"><div><p class="kicker">ATENÇÃO</p><h2>Alertas operacionais</h2></div></div>
      <RouterLink v-for="alert in data.alerts" :key="alert.code" class="alert-row" :to="alert.action_url || '/'"><strong>{{ alert.title }}</strong><span>{{ alert.description }}</span></RouterLink>
    </section>
    <section v-if="data.host" class="panel panel-section resource-health"><div class="section-head"><div><p class="kicker">SAÚDE DO SERVIDOR</p><h2>Recursos do ambiente</h2></div><button class="ghost" type="button" @click="load()">Atualizar</button></div><div class="resource-bars"><div><span>CPU</span><strong>{{ data.host.cpu_percent == null ? "Indisponível" : `${data.host.cpu_percent}%` }}</strong></div><div><span>Memória</span><strong>{{ data.host.memory_used_bytes == null ? "Indisponível" : `${Math.round(data.host.memory_used_bytes / 1073741824 * 10) / 10} GB de ${Math.round(data.host.memory_total_bytes / 1073741824 * 10) / 10} GB` }}</strong></div><div><span>Disco</span><strong>{{ data.host.disk_used_bytes == null ? "Indisponível" : `${Math.round(data.host.disk_used_bytes / 1073741824)} GB de ${Math.round(data.host.disk_total_bytes / 1073741824)} GB` }}</strong></div></div></section>
    <section v-if="data.deployments?.recent?.length" class="panel panel-section operational-alerts"><div class="section-head"><div><p class="kicker">DEPLOYS</p><h2>Deploys recentes</h2></div><RouterLink class="ghost" to="/activity">Ver atividade</RouterLink></div><RouterLink v-for="item in data.deployments.recent" :key="item.id" class="alert-row" :to="`/applications/${item.application_id}?tab=deployments`"><strong>{{ item.application }} · {{ item.status }}</strong><span>Revisão {{ item.revision }} · {{ new Date(item.created_at).toLocaleString("pt-BR") }}</span></RouterLink></section>
    <section v-if="infra.available && !infra.swarm?.active" class="callout">
      <div>
        <p class="kicker">PRÓXIMO PASSO</p>
        <h2>Prepare o ambiente para publicar aplicações</h2>
        <p class="muted">
          O Docker Swarm organiza atualizações e permite administrar serviços
          com segurança.
        </p>
      </div>
      <RouterLink class="primary" to="/infrastructure"
        >Preparar ambiente</RouterLink
      >
    </section>
    <div class="grid metrics">
      <article>
        <span class="label">PROJETOS</span><strong>{{ data.projects }}</strong>
        <p class="muted">Ambientes organizados</p>
      </article>
      <article>
        <span class="label">APLICAÇÕES</span
        ><strong>{{ data.applications }}</strong>
        <p class="muted">Em todos os projetos</p>
      </article>
      <article>
        <span class="label">CONTAINERS</span
        ><strong>{{ infra.containers ?? "Não disponível" }}</strong>
        <p class="muted">{{ infra.running || 0 }} em execução</p>
      </article>
      <article>
        <span class="label">IMAGENS</span
        ><strong>{{ infra.images ?? "Não disponível" }}</strong>
        <p class="muted">Disponíveis localmente</p>
      </article>
      <article>
        <span class="label">VOLUMES</span
        ><strong>{{ infra.volumes ?? "Não disponível" }}</strong>
        <p class="muted">Dados persistentes</p>
      </article>
      <article>
        <span class="label">REDES</span
        ><strong>{{ infra.networks ?? "Não disponível" }}</strong>
        <p class="muted">Conexões Docker</p>
      </article>
    </div>
    <section class="resource-strip">
      <div>
        <span class="label">STACKS</span
        ><strong>{{ infra.stacks ?? "Não disponível" }}</strong>
      </div>
      <div>
        <span class="label">NÓS DO SWARM</span
        ><strong>{{
          infra.swarm?.active ? infra.swarm.nodes : "Não disponível"
        }}</strong>
      </div>
      <div>
        <span class="label">SERVIÇOS</span
        ><strong>{{
          infra.swarm?.active ? infra.swarm.services : "Não disponível"
        }}</strong>
      </div>
      <div>
        <span class="label">TASKS COM FALHA</span
        ><strong>{{
          infra.swarm?.active ? infra.swarm.tasks_failed : "Não disponível"
        }}</strong>
      </div>
    </section>
    <div class="split-panels">
      <section class="panel panel-section">
        <div class="section-head">
          <div>
            <p class="kicker">PROJETOS</p>
            <h2>Projetos recentes</h2>
          </div>
          <RouterLink class="ghost" to="/projects">Ver todos</RouterLink>
        </div>
        <div v-if="!data.projects" class="small-empty">
          <p>Nenhum projeto criado ainda.</p>
          <button class="secondary" type="button" @click="showProject = true">
            Criar primeiro projeto
          </button>
        </div>
        <div v-else class="project-list">
          <RouterLink
            v-for="project in data.recent_projects || []"
            :key="project.id"
            :to="`/projects/${project.id}`"
            ><strong>{{ project.name }}</strong
            ><span
              >{{ project.applications_count || 0 }} aplicações</span
            ></RouterLink
          >
        </div>
      </section>
      <section class="panel panel-section">
        <div class="section-head">
          <div>
            <p class="kicker">REGISTRO</p>
            <h2>Atividade recente</h2>
          </div>
          <RouterLink class="ghost" to="/activity">Ver tudo</RouterLink>
        </div>
        <div v-if="!data.activity?.length" class="small-empty">
          <p>As ações importantes aparecerão aqui.</p>
        </div>
        <div v-else class="project-list">
          <article
            v-for="item in data.activity"
            :key="item.created_at + item.action"
          >
            <strong>{{ item.description || item.action }}</strong
            ><span>{{
              new Date(item.created_at).toLocaleDateString("pt-BR")
            }}</span>
          </article>
        </div>
      </section>
    </div>
  </template>
  <BaseModal
    :open="showProject"
    title="Novo projeto"
    description="Organize aplicações e ambientes em um só lugar."
    @close="showProject = false"
    ><form id="dashboard-project-form" @submit.prevent="createProject">
      <label
        >Nome<input
          v-model="fields.name"
          required
          autofocus
          placeholder="Ex.: Site institucional"
        /><small v-if="fieldErrors.name" class="error-field">{{
          fieldErrors.name
        }}</small></label
      ><label
        >Descrição <span class="muted">(opcional)</span
        ><textarea
          v-model="fields.description"
          placeholder="Para que este projeto serve?"
        ></textarea>
      </label>
    </form>
    <template #footer
      ><button
        class="secondary"
        type="button"
        :disabled="saving"
        @click="showProject = false"
      >
        Cancelar</button
      ><button
        class="primary"
        form="dashboard-project-form"
        type="submit"
        :disabled="saving"
      >
        {{ saving ? "Criando…" : "Criar projeto" }}
      </button></template
    ></BaseModal
  >
</template>

<style scoped>
.operational-cards { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.operational-card { display: grid; gap: 8px; color: inherit; text-decoration: none; }
.operational-card strong { font-size: 28px; }
.operational-card span:last-child, .alert-row span { color: var(--muted, #858b99); font-size: 13px; }
.operational-alerts { display: grid; gap: 12px; }
.alert-row { display: flex; justify-content: space-between; gap: 16px; padding: 12px 0; border-top: 1px solid #2a2d34; color: inherit; text-decoration: none; }
.resource-bars { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; }
.resource-bars > div { display: grid; gap: 6px; }
.resource-bars span { color: var(--muted, #858b99); font-size: 12px; }
@media (max-width: 800px) { .operational-cards, .resource-bars { grid-template-columns: 1fr 1fr; } .alert-row { display: grid; gap: 4px; } }
@media (max-width: 480px) { .operational-cards, .resource-bars { grid-template-columns: 1fr; } }
</style>
