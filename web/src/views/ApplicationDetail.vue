<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import BaseModal from "../components/ui/BaseModal.vue";
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
};
type SourceData = {
  source_type: string;
  configured: boolean;
  payload?: Record<string, any>;
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
  };
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
const newVariable = ref<Variable>({ key: "", value: "", secret: false });
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
    await loadSource();
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
      `/api/v1/applications/${application.value?.id}/source/validate`,
      { method: "POST" },
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
function addVariable() {
  if (!newVariable.value.key.trim()) return;
  variables.value.push({ ...newVariable.value });
  newVariable.value = { key: "", value: "", secret: false };
}
function removeVariable(index: number) {
  variables.value.splice(index, 1);
}
function setTab(value: string) {
  router.replace({
    query: { ...route.query, tab: value === "overview" ? undefined : value },
  });
}
watch(() => route.params.applicationId, load);
watch(
  () => application.value?.name,
  (name) => {
    if (name) document.title = `${name} · StackHost`;
  },
  { immediate: true },
);
onMounted(load);
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
        </div>
        <p v-else class="muted">
          Valide a origem para exibir serviços, imagens, portas, volumes e redes
          detectados.
        </p>
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
        <label
          >Docker Compose<textarea
            v-model="composeYaml"
            class="code-editor"
            spellcheck="false"
            placeholder="services:\n  web:\n    image: nginx:1.27-alpine"
          />
          <input
            type="file"
            accept=".yml,.yaml,text/yaml"
            @change="readComposeFile"
          />
        </label>
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
    <section v-else-if="tab === 'variables'" class="panel variable-editor">
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
      <div class="variable-row new-variable">
        <input v-model="newVariable.key" placeholder="CHAVE" /><input
          v-model="newVariable.value"
          placeholder="Valor"
          :type="newVariable.secret ? 'password' : 'text'"
        /><label class="check"
          ><input v-model="newVariable.secret" type="checkbox" /> Secret</label
        ><button class="secondary" type="button" @click="addVariable">
          Adicionar
        </button>
      </div>
      <div v-if="!variables.length" class="empty compact-empty">
        <p>Nenhuma variável configurada.</p>
      </div>
      <div
        v-for="(item, index) in variables"
        :key="`${item.key}-${index}`"
        class="variable-row"
      >
        <strong>{{ item.key }}</strong
        ><span>{{
          item.secret
            ? item.has_value
              ? "•••••••• · valor configurado"
              : "Sem valor"
            : item.value
        }}</span
        ><button class="ghost" type="button" @click="removeVariable(index)">
          Remover
        </button>
      </div>
    </section>
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
