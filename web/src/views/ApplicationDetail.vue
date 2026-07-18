<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import BaseModal from "../components/ui/BaseModal.vue";
import ComposeCodeEditor from "../components/applications/ComposeCodeEditor.vue";
import DomainsPanel from "../components/applications/DomainsPanel.vue";
import DeploymentTimeline from "../components/applications/DeploymentTimeline.vue";
import EnvironmentVariablesEditor from "../components/applications/EnvironmentVariablesEditor.vue";
import LogsPanel from "../components/applications/LogsPanel.vue";
import { api, RequestError } from "../composables/useApi";
import { useToast } from "../composables/useToast";

type Variable = {
  key: string;
  value: string;
  secret: boolean;
  has_value?: boolean;
};
type ApplicationData = {
  id: number;
  name: string;
  description: string;
  slug: string;
  source_type: string;
  docker_stack_name: string;
  configuration_status: string;
  source_revision: number;
  project: { id: number; name: string };
  activity: Array<{ description: string; created_at: string }>;
  source_summary?: { environment_variables?: Array<{ name: string; default_value?: string; required?: boolean; secret?: boolean; services?: string[] }> };
};
type SourceData = {
  source_type: string;
  configured: boolean;
  payload?: Record<string, any>;
  summary?: ApplicationData["source_summary"];
};
type Validation = {
  valid: boolean;
  errors: string[];
  warnings: string[];
  summary: {
    services: string[];
    images: string[];
    ports: string[];
    volumes: string[];
    networks: string[];
    environment_variables?: Array<{ name: string; default_value?: string; required?: boolean; secret?: boolean; services?: string[] }>;
  };
};
type Runtime = {
  mode: string;
  status: string;
  services: Array<{
    name: string;
    image: string;
    desired: number;
    running: number;
    failed: number;
    health?: string;
  }>;
};
type Metrics = {
  available: boolean;
  runtime_mode: string;
  current: Array<{ service: string; cpu_percent: number; memory_bytes: number; memory_limit_bytes: number }>;
  history: Array<{ service: string; cpu_percent: number; memory_bytes: number; recorded_at: string }>;
};

const route = useRoute();
const router = useRouter();
const toast = useToast();
const application = ref<ApplicationData | null>(null);
const source = ref<SourceData | null>(null);
const validation = ref<Validation | null>(null);
const loading = ref(true);
const error = ref("");
const saving = ref(false);
const publishing = ref(false);
const tab = computed(() =>
  typeof route.query.tab === "string" ? route.query.tab : "overview",
);
const editOpen = ref(false);
const changeOpen = ref(false);
const meta = ref({ name: "", description: "", docker_stack_name: "" });
const sourceType = ref("compose");
const pendingSourceType = ref("");
const composeYaml = ref("");
const image = ref("");
const containerPort = ref(0);
const replicas = ref(1);
const repositoryUrl = ref("");
const branch = ref("main");
const dockerfilePath = ref("Dockerfile");
const buildContext = ref(".");
const variables = ref<Variable[]>([]);
const detectedVariables = ref<NonNullable<Validation["summary"]["environment_variables"]>>([]);
const runtime = ref<Runtime | null>(null);
const metrics = ref<Metrics | null>(null);
const deploymentsRefreshKey = ref(0);
const sourceTypes = [
  { value: "compose", label: "Docker Compose" },
  { value: "image", label: "Imagem Docker" },
  { value: "git", label: "Repositório Git" },
  { value: "catalog", label: "Catálogo" },
];

function sourceLabel(value: string) {
  return sourceTypes.find((item) => item.value === value)?.label || value;
}
function stateLabel(value: string) {
  return (
    { draft: "Rascunho", configured: "Configurada", invalid: "Requer atenção" }[
      value
    ] || "Rascunho"
  );
}
async function load() {
  loading.value = true;
  error.value = "";
  try {
    application.value = await api<ApplicationData>(
      `/api/v1/applications/${route.params.applicationId}`,
    );
    detectedVariables.value = application.value.source_summary?.environment_variables || [];
    await loadSource();
    runtime.value = await api<Runtime>(`/api/v1/applications/${route.params.applicationId}/runtime`);
    metrics.value = await api<Metrics>(`/api/v1/applications/${route.params.applicationId}/metrics`).catch(() => null);
  } catch (err) {
    error.value =
      err instanceof Error
        ? err.message
        : "Não foi possível carregar a aplicação.";
  } finally {
    loading.value = false;
  }
}
async function readComposeFile(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (file) composeYaml.value = await file.text();
}
async function loadSource() {
  source.value = await api<SourceData>(
    `/api/v1/applications/${route.params.applicationId}/source`,
  );
  sourceType.value =
    source.value.source_type || application.value?.source_type || "compose";
  if (!detectedVariables.value.length) {
    detectedVariables.value = source.value.summary?.environment_variables || [];
  }
  const payload = source.value.payload || {};
  composeYaml.value = payload.compose_yaml || "";
  image.value = payload.image || "";
  containerPort.value = payload.container_port || 0;
  replicas.value = payload.replicas || 1;
  repositoryUrl.value = payload.repository_url || "";
  branch.value = payload.branch || "main";
  dockerfilePath.value = payload.dockerfile_path || "Dockerfile";
  buildContext.value = payload.build_context || ".";
  variables.value = payload.environment || [];
}
function openMeta() {
  if (!application.value) return;
  meta.value = {
    name: application.value.name,
    description: application.value.description || "",
    docker_stack_name: application.value.docker_stack_name || "",
  };
  editOpen.value = true;
}
async function saveMeta() {
  saving.value = true;
  try {
    await api(`/api/v1/applications/${application.value?.id}`, {
      method: "PATCH",
      body: JSON.stringify(meta.value),
    });
    editOpen.value = false;
    toast.success("Aplicação atualizada.");
    await load();
  } catch (err) {
    toast.error(
      err instanceof Error ? err.message : "Não foi possível salvar.",
    );
  } finally {
    saving.value = false;
  }
}
function payload() {
  return {
    compose_yaml: composeYaml.value,
    image: image.value,
    container_port: containerPort.value,
    replicas: replicas.value,
    repository_url: repositoryUrl.value,
    branch: branch.value,
    dockerfile_path: dockerfilePath.value,
    build_context: buildContext.value,
    environment: variables.value,
  };
}
async function saveSource() {
  saving.value = true;
  try {
    await api(`/api/v1/applications/${application.value?.id}/source`, {
      method: "PUT",
      body: JSON.stringify({
        source_type: sourceType.value,
        source: payload(),
      }),
    });
    toast.success("Origem salva com segurança.");
    await load();
  } catch (err) {
    toast.error(
      err instanceof RequestError
        ? err.message
        : "Não foi possível salvar a origem.",
    );
  } finally {
    saving.value = false;
  }
}
async function validateSource() {
  saving.value = true;
  try {
    validation.value = await api<Validation>(
      "/api/v1/source/validate",
      {
        method: "POST",
        body: JSON.stringify({ source_type: sourceType.value, source: payload() }),
      },
    );
    toast[validation.value.valid ? "success" : "error"](
      validation.value.valid
        ? "Configuração válida."
        : "Corrija os problemas encontrados.",
    );
  } catch (err) {
    toast.error(
      err instanceof Error ? err.message : "Não foi possível validar.",
    );
  } finally {
    saving.value = false;
  }
}
async function runtimeAction(operation: "start" | "stop" | "restart" | "remove") {
  const destructive = operation === "stop" || operation === "remove";
  if (destructive && !window.confirm(operation === "remove" ? "Remover a publicação do Docker? A aplicação continuará no StackHost." : "Parar esta publicação agora?")) return;
  try {
    await api(`/api/v1/applications/${route.params.applicationId}/${operation === "remove" ? "runtime" : `actions/${operation}`}`, { method: operation === "remove" ? "DELETE" : "POST" });
    toast.success(operation === "remove" ? "Publicação removida." : "Ação enviada ao runtime.");
    await load();
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "Não foi possível alterar o runtime.");
  }
}
async function duplicateApplication() {
  try {
    const copy = await api<{ id: number }>(`/api/v1/applications/${route.params.applicationId}/duplicate`, { method: "POST" });
    toast.success("Cópia criada.");
    router.push(`/applications/${copy.id}`);
  } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível duplicar a aplicação."); }
}
async function publishApplication() {
  const mode = runtime.value?.mode === "swarm" ? "Docker Swarm" : "Docker em servidor único";
  if (!window.confirm(`Publicar aplicação?\n\nModo: ${mode}\nProjeto: ${application.value?.slug || application.value?.name || "aplicação"}`)) return;
  publishing.value = true;
  try {
    await api(
      `/api/v1/applications/${route.params.applicationId}/deployments`,
      {
        method: "POST",
        body: JSON.stringify({ source_revision: application.value?.source_revision }),
      },
    );
    deploymentsRefreshKey.value += 1;
    toast.success("Publicação enfileirada. Acompanhe o progresso em Deploys.");
    setTab("deployments");
  }
  catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível publicar a aplicação."); }
  finally { publishing.value = false; }
}
async function changeSource() {
  saving.value = true;
  try {
    await api(`/api/v1/applications/${application.value?.id}/source/change`, {
      method: "POST",
      body: JSON.stringify({
        source_type: pendingSourceType.value,
        confirm_reset: true,
      }),
    });
    changeOpen.value = false;
    toast.success("Origem alterada. Configure a nova origem.");
    await load();
    setTab("source");
  } catch (err) {
    toast.error(
      err instanceof Error ? err.message : "Não foi possível trocar a origem.",
    );
  } finally {
    saving.value = false;
  }
}
function requestSourceChange(value: string) {
  if (value === sourceType.value) return;
  pendingSourceType.value = value;
  changeOpen.value = true;
}
function setTab(value: string) {
  router.replace({
    query: { ...route.query, tab: value === "overview" ? undefined : value },
  });
}
watch(
  () => route.params.applicationId,
  () => {
    void load();
  },
);
watch(
  () => application.value?.name,
  (name) => {
    if (name) document.title = `${name} · StackHost`;
  },
  { immediate: true },
);
onMounted(() => {
  void load();
});
</script>

<template>
  <div v-if="loading" class="empty">
    <span class="skeleton" />
    <p>Carregando aplicação…</p>
  </div>
  <div v-else-if="error" class="empty">
    <h2>Não foi possível carregar</h2>
    <p>{{ error }}</p>
    <button class="secondary" type="button" @click="load">
      Tentar novamente
    </button>
  </div>
  <template v-else-if="application">
    <div class="project-breadcrumb">
      <RouterLink :to="`/projects/${application.project.id}`">{{
        application.project.name
      }}</RouterLink
      ><span>/</span><strong>{{ application.name }}</strong>
    </div>
    <div class="hero application-hero">
      <div>
        <p class="kicker">APLICAÇÃO</p>
        <h1>{{ application.name }}</h1>
        <p class="muted">
          {{
            application.description ||
            "Configure a origem para preparar esta aplicação."
          }}
        </p>
        <div class="application-meta">
          <span
            class="status-badge"
            :class="application.configuration_status"
            >{{ stateLabel(application.configuration_status) }}</span
          ><span>{{ sourceLabel(application.source_type) }}</span
          ><span>Revisão {{ application.source_revision }}</span>
        </div>
      </div>
      <button class="secondary" type="button" @click="openMeta">
        Editar aplicação
      </button>
    </div>
    <div class="tabs application-tabs">
      <button
        v-for="item in [
          { value: 'overview', label: 'Visão geral' },
          { value: 'source', label: 'Origem' },
          { value: 'variables', label: 'Variáveis' },
          { value: 'domains', label: 'Domínios' },
          { value: 'deployments', label: 'Deploys' },
          { value: 'logs', label: 'Logs' },
          { value: 'activity', label: 'Atividade' },
        ]"
        :key="item.value"
        :class="{ active: tab === item.value }"
        type="button"
        @click="setTab(item.value)"
      >
        {{ item.label }}
      </button>
    </div>
    <section v-if="tab === 'overview'" class="detail-grid">
      <article class="panel">
        <p class="label">CONFIGURAÇÃO</p>
        <h2>{{ stateLabel(application.configuration_status) }}</h2>
        <p class="muted">
          {{
            application.configuration_status === "configured"
              ? "Configuração pronta para publicação."
              : "Conclua a configuração da origem para continuar."
          }}
        </p>
        <button
          v-if="application.configuration_status !== 'configured'"
          class="primary"
          type="button"
          @click="setTab('source')"
        >
          Concluir configuração
        </button>
      </article>
      <article class="panel">
        <p class="label">ORIGEM</p>
        <h2>{{ sourceLabel(application.source_type) }}</h2>
        <p class="muted">
          {{
            source?.configured
              ? "Origem configurada e protegida."
              : "Nenhuma configuração salva."
          }}
        </p>
        <button class="secondary" type="button" @click="setTab('source')">
          Configurar origem
        </button>
      </article>
      <article class="panel full-width runtime-panel">
        <p class="label">RUNTIME</p>
        <h2>{{ runtime?.status || "unknown" }}</h2>
        <div class="runtime-actions">
          <button v-if="runtime?.status === 'stopped'" class="secondary" type="button" @click="runtimeAction('start')">Iniciar</button>
          <button v-if="runtime?.status === 'running' || runtime?.status === 'degraded'" class="secondary" type="button" @click="runtimeAction('stop')">Parar</button>
          <button v-if="runtime?.status === 'running' || runtime?.status === 'degraded'" class="secondary" type="button" @click="runtimeAction('restart')">Reiniciar</button>
          <button v-if="runtime?.status === 'running' || runtime?.status === 'stopped' || runtime?.status === 'degraded'" class="ghost" type="button" @click="runtimeAction('remove')">Remover publicação</button>
          <button class="ghost" type="button" @click="duplicateApplication">Duplicar aplicação</button>
        </div>
      </article>
      <article class="panel full-width">
        <p class="label">RESUMO VALIDADO</p>
        <div v-if="validation" class="summary-grid compact">
          <div>
            <span>Serviços</span
            ><strong>{{ validation.summary.services.length }}</strong>
          </div>
          <div>
            <span>Imagens</span
            ><strong>{{ validation.summary.images.length }}</strong>
          </div>
          <div>
            <span>Portas</span
            ><strong>{{ validation.summary.ports.length }}</strong>
          </div>
          <div>
            <span>Variáveis</span
            ><strong>{{ detectedVariables.length }} detectadas</strong>
          </div>
        </div>
          <p v-else class="muted">
          Valide a origem para exibir serviços, imagens, portas, volumes e redes
          detectados.
        </p>
      </article>
      <article v-if="sourceType === 'compose' && runtime" class="panel full-width">
        <p class="label">PUBLICAÇÃO</p>
        <h2>{{ runtime.status === "running" ? "Aplicação em execução" : "Ainda não publicada" }}</h2>
        <p class="muted">Modo: {{ runtime.mode === "swarm" ? "Docker Swarm" : "Docker em servidor único" }}</p>
        <div v-if="runtime.services.length" class="summary-grid compact"><div v-for="service in runtime.services" :key="service.name"><span>{{ service.name }}</span><strong>{{ service.running }}/{{ service.desired }}</strong><small>{{ service.image }}</small><small v-if="service.health">Health: {{ service.health }}</small></div></div>
      </article>
      <article v-if="metrics?.available && metrics.current.length" class="panel full-width">
        <p class="label">MÉTRICAS LOCAIS</p>
        <p class="muted">Amostra atual do Docker neste host · retenção de 24 horas.</p>
        <div class="summary-grid compact"><div v-for="metric in metrics.current" :key="metric.service"><span>{{ metric.service }}</span><strong>{{ metric.cpu_percent.toFixed(1) }}% CPU</strong><small>{{ Math.round(metric.memory_bytes / 1048576) }} MB de memória</small></div></div>
      </article>
    </section>
    <section v-else-if="tab === 'source'" class="source-editor">
      <div class="section-head">
        <div>
          <p class="kicker">CONFIGURAÇÃO DA ORIGEM</p>
          <h2>Como a aplicação será construída</h2>
        </div>
        <div class="hero-actions">
          <button
            class="secondary"
            type="button"
            :disabled="saving"
            @click="validateSource"
          >
            Validar</button
          ><button
            class="primary"
            type="button"
            :disabled="saving"
            @click="saveSource"
          >
            {{ saving ? "Salvando…" : "Salvar alterações" }}
          </button>
          <button v-if="sourceType === 'compose'" class="secondary" type="button" :disabled="publishing" @click="publishApplication">{{ publishing ? "Publicando…" : "Publicar aplicação" }}</button>
        </div>
      </div>
      <div class="source-switch">
        <button
          v-for="item in sourceTypes"
          :key="item.value"
          type="button"
          :class="{ selected: sourceType === item.value }"
          @click="requestSourceChange(item.value)"
        >
          {{ item.label }}
        </button>
      </div>
      <div v-if="sourceType === 'compose'">
        <label>Docker Compose</label>
        <ComposeCodeEditor
          v-model="composeYaml"
          @validate="validateSource"
          @save="saveSource"
          @error="toast.error"
        />
      </div>
      <div v-else-if="sourceType === 'image'" class="form-grid">
        <label
          >Imagem Docker<input
            v-model="image"
            placeholder="nginx:1.27-alpine" /></label
        ><label
          >Porta interna<input
            v-model.number="containerPort"
            type="number"
            min="0"
            max="65535" /></label
        ><label
          >Réplicas<input v-model.number="replicas" type="number" min="1"
        /></label>
      </div>
      <div v-else-if="sourceType === 'git'" class="form-grid">
        <label
          >URL do repositório<input
            v-model="repositoryUrl"
            placeholder="https://github.com/org/repo.git" /></label
        ><label>Branch<input v-model="branch" /></label
        ><label>Dockerfile<input v-model="dockerfilePath" /></label
        ><label>Contexto<input v-model="buildContext" /></label>
      </div>
      <div v-else class="empty compact-empty">
        <h2>Catálogo oficial</h2>
        <p>Selecione um template no catálogo durante a criação da aplicação.</p>
      </div>
      <div
        v-if="validation"
        class="validation-result"
        :class="{ invalid: !validation.valid }"
      >
        <strong>{{
          validation.valid ? "Configuração válida" : "Requer atenção"
        }}</strong>
        <p
          v-for="item in [...validation.errors, ...validation.warnings]"
          :key="item"
        >
          {{ item }}
        </p>
      </div>
    </section>
    <section v-else-if="tab === 'variables'" class="variable-editor">
      <div class="section-head">
        <div>
          <p class="kicker">VARIÁVEIS</p>
          <h2>Valores da aplicação</h2>
        </div>
        <button
          class="primary"
          type="button"
          :disabled="saving"
          @click="saveSource"
        >
          Salvar variáveis
        </button>
      </div>
      <EnvironmentVariablesEditor v-model="variables" :detected="detectedVariables" />
    </section>
    <DomainsPanel
      v-else-if="tab === 'domains'"
      :application-id="application.id"
      :services="runtime?.services.map((service) => service.name) || []"
    />
    <DeploymentTimeline
      v-else-if="tab === 'deployments'"
      :application-id="application.id"
      :publishing="publishing"
      :refresh-key="deploymentsRefreshKey"
      @publish="publishApplication"
    />
    <LogsPanel
      v-else-if="tab === 'logs'"
      :application-id="application.id"
    />
    <section v-else class="activity-list">
      <article
        v-for="item in application.activity"
        :key="`${item.created_at}-${item.description}`"
      >
        <div class="timeline-icon">•</div>
        <div class="activity-copy">
          <strong>{{ item.description }}</strong>
          <p class="muted">
            {{ new Date(item.created_at).toLocaleString("pt-BR") }}
          </p>
        </div>
      </article>
      <div v-if="!application.activity.length" class="empty">
        <p>Nenhuma atividade desta aplicação ainda.</p>
      </div>
    </section>
  </template>
  <BaseModal
    :open="editOpen"
    title="Editar aplicação"
    description="Atualize somente os metadados da aplicação."
    @close="editOpen = false"
    ><form id="application-meta-form" @submit.prevent="saveMeta">
      <label>Nome<input v-model="meta.name" required /></label
      ><label>Descrição<textarea v-model="meta.description" /></label
      ><label
        >Nome futuro da stack<input v-model="meta.docker_stack_name"
      /></label>
    </form>
    <template #footer
      ><button class="secondary" type="button" @click="editOpen = false">
        Cancelar</button
      ><button
        class="primary"
        form="application-meta-form"
        type="submit"
        :disabled="saving"
      >
        Salvar
      </button></template
    ></BaseModal
  >
  <BaseModal
    :open="changeOpen"
    title="Trocar origem"
    description="A configuração atual será removida com segurança."
    @close="changeOpen = false"
    ><p class="muted">
      Confirme somente se deseja começar uma nova configuração como
      {{ sourceLabel(pendingSourceType) }}.
    </p>
    <template #footer
      ><button class="secondary" type="button" @click="changeOpen = false">
        Cancelar</button
      ><button
        class="primary"
        type="button"
        :disabled="saving"
        @click="changeSource"
      >
        Confirmar troca
      </button></template
    ></BaseModal
  >
</template>

<style scoped>
.runtime-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
.deployments-section {
  display: grid;
  gap: 20px;
}
.deployments-head {
  align-items: center;
  margin-bottom: 0;
}
.deployments-head h2 {
  margin-top: 0;
}
.deployments-head p {
  margin: 6px 0 0;
}
.deployment-loading {
  overflow: hidden;
  border: 1px solid #2a2d34;
  border-radius: 10px;
  background: #17191e;
}
.deployment-skeleton {
  display: grid;
  gap: 12px;
  padding: 22px;
  border-bottom: 1px solid #2a2d34;
}
.deployment-skeleton:last-child {
  border-bottom: 0;
}
.deployment-skeleton .skeleton {
  max-width: 340px;
}
.deployment-skeleton .short {
  max-width: 210px;
  height: 14px;
}
.deployment-empty {
  margin: 0;
  padding: 48px 20px;
}
.deployment-inline-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 14px;
  border: 1px solid #633c42;
  border-radius: 8px;
  background: #2a1c20;
  color: #efb3b3;
  font-size: 13px;
}
.deployment-inline-error .ghost {
  flex: 0 0 auto;
}
.deployment-timeline {
  display: grid;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.deployment-timeline > li {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  gap: 12px;
}
.deployment-rail {
  position: relative;
  display: flex;
  justify-content: center;
}
.deployment-rail::after {
  position: absolute;
  top: 27px;
  bottom: -17px;
  width: 1px;
  background: #343842;
  content: "";
}
.deployment-timeline > li:last-child .deployment-rail::after {
  display: none;
}
.deployment-marker {
  z-index: 1;
  width: 12px;
  height: 12px;
  margin-top: 21px;
  border-radius: 50%;
  background: #858b99;
  outline: 5px solid #101114;
}
.deployment-entry {
  min-width: 0;
  padding: 18px 20px;
  border: 1px solid #2a2d34;
  border-radius: 10px;
  background: #17191e;
}
.deployment-entry-head,
.deployment-title {
  display: flex;
  align-items: center;
  gap: 10px;
}
.deployment-entry-head {
  justify-content: space-between;
}
.deployment-entry-head time {
  color: #858b99;
  font-size: 12px;
}
.deployment-title strong {
  font-size: 14px;
}
.deployment-status {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 5px 9px;
  background: #262a30;
  color: #c7ccd6;
  font-size: 12px;
  font-weight: 600;
}
.deployment-message {
  margin: 14px 0 0;
  color: #b9bec9;
  line-height: 1.5;
}
.deployment-meta-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 14px 28px;
  margin: 16px 0 0;
  padding-top: 14px;
  border-top: 1px solid #2a2d34;
}
.deployment-meta-grid div {
  display: grid;
  min-width: 110px;
  gap: 4px;
  padding: 0;
  border: 0;
}
.deployment-meta-grid dt {
  color: #737985;
  font-size: 11px;
}
.deployment-meta-grid dd {
  color: #d7dbe3;
  font-size: 12px;
  text-align: left;
}
.deployment-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
.deployment-actions .secondary {
  padding: 8px 12px;
  font-size: 13px;
}
.deployment-output {
  margin-top: 16px;
  padding-top: 14px;
}
.deployment-output p {
  margin: 12px 0 0;
  color: #efb3b3;
  font-size: 12px;
}
.deployment-output pre {
  max-height: 320px;
  overflow: auto;
  margin: 12px 0 0;
  padding: 14px;
  border-radius: 7px;
  background: #111317;
  color: #d7dbe3;
  font: 12px/1.6 "Space Mono", monospace;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
.deployment-marker.status-preparing,
.deployment-marker.status-interrupted {
  background: #e0ad55;
}
.deployment-status.status-preparing,
.deployment-status.status-interrupted {
  background: #2d281b;
  color: #e7c785;
}
.deployment-marker.status-deploying,
.deployment-marker.status-waiting {
  background: #e8f55b;
}
.deployment-status.status-deploying,
.deployment-status.status-waiting {
  background: #252a16;
  color: #e8f55b;
}
.deployment-marker.status-succeeded {
  background: #71c58a;
}
.deployment-status.status-succeeded {
  background: #1c2b22;
  color: #92d5a5;
}
.deployment-marker.status-failed {
  background: #ef8f8f;
}
.deployment-status.status-failed {
  background: #2a1c20;
  color: #efb3b3;
}
.deployment-marker.status-cancelled {
  background: #737985;
}
@media (max-width: 700px) {
  .deployments-head,
  .deployment-entry-head {
    align-items: stretch;
    flex-direction: column;
  }
  .deployments-head .primary {
    width: 100%;
  }
  .deployment-timeline > li {
    grid-template-columns: 18px minmax(0, 1fr);
    gap: 7px;
  }
  .deployment-entry {
    padding: 16px;
  }
  .deployment-title {
    flex-wrap: wrap;
  }
  .deployment-meta-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .deployment-meta-grid div {
    min-width: 0;
  }
  .deployment-actions .secondary {
    width: 100%;
  }
  .deployment-inline-error {
    align-items: stretch;
    flex-direction: column;
  }
}
@media (max-width: 420px) {
  .deployment-meta-grid {
    grid-template-columns: 1fr;
  }
}
@media (prefers-reduced-motion: reduce) {
  .deployment-skeleton .skeleton {
    animation: none;
  }
}
</style>
