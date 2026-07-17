<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import BaseModal from "../components/ui/BaseModal.vue";
import ComposeCodeEditor from "../components/applications/ComposeCodeEditor.vue";
import ApplicationWizard from "../components/applications/ApplicationWizard.vue";
import SourceSummary from "../components/applications/SourceSummary.vue";
import EnvironmentVariablesEditor from "../components/applications/EnvironmentVariablesEditor.vue";
import SourceValidationPanel from "../components/applications/SourceValidationPanel.vue";
import { api, RequestError } from "../composables/useApi";
import { useToast } from "../composables/useToast";
import type { Application, Project } from "../types";

const route = useRoute();
const router = useRouter();
const toast = useToast();
const project = ref<Project | null>(null);
const apps = ref<Application[]>([]);
const loading = ref(true);
const error = ref("");
const projectModal = ref(false);
const appModal = ref(false);
const wizardDiscardOpen = ref(false);
const sourceChangeOpen = ref(false);
const pendingSourceType = ref("");
const deleteApp = ref<Application | null>(null);
const saving = ref(false);
const validating = ref(false);
const validationPreview = ref<{
  valid: boolean;
  errors: string[];
  warnings: string[];
  summary?: { services: string[]; images: string[]; ports: string[] };
} | null>(null);
const fieldErrors = ref<Record<string, string>>({});
const editProject = ref({ name: "", description: "" });
const editApp = ref({ id: 0, name: "" });
const newApp = ref({
  name: "",
  description: "",
  docker_stack_name: "",
  source_type: "",
});
const appStep = ref(1);
const newSource = ref({
  compose_yaml: "",
  image: "",
  container_port: null as number | null,
  replicas: 1,
  repository_url: "",
  branch: "main",
  dockerfile_path: "Dockerfile",
  build_context: ".",
  command: "",
  entrypoint: "",
  environment: [] as Array<{ key: string; value: string; secret: boolean }>,
  template_slug: "",
  template_version: "",
});
const catalogTemplates = ref<
  Array<{
    slug: string;
    name: string;
    description: string;
    version: string;
    category: string;
    source: string;
  }>
>([]);
const composeSummary = computed(() => {
  const yaml = newSource.value.compose_yaml;
  return {
    services: [...yaml.matchAll(/^\s{2}([\w.-]+):\s*$/gm)].map(
      (match) => match[1],
    ),
    images: [...yaml.matchAll(/^\s+image:\s*([^\s#]+)/gm)].map(
      (match) => match[1],
    ),
    ports: [...yaml.matchAll(/^\s+-\s*["']?([^"']+)["']?\s*$/gm)]
      .map((match) => match[1])
      .filter((port) => port.includes(":")),
    volumes: [...yaml.matchAll(/^\s+volumes:\s*$/gm)].map(() => "volume"),
    networks: [...yaml.matchAll(/^\s+networks:\s*$/gm)].map(() => "network"),
  };
});
const selectedCatalog = computed(() =>
  catalogTemplates.value.find(
    (template) => template.slug === newSource.value.template_slug,
  ),
);
const slugPreview = computed(() =>
  newApp.value.name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 100) || "nome-da-aplicacao",
);
const catalogCategories = computed(() => [
  ...new Set(catalogTemplates.value.map((template) => template.category)),
]);
const catalogQuery = ref("");
const catalogCategory = ref("all");
const visibleCatalogTemplates = computed(() => {
  const query = catalogQuery.value.trim().toLowerCase();
  return catalogTemplates.value.filter(
    (template) =>
      (!query || `${template.name} ${template.description}`.toLowerCase().includes(query)) &&
      (catalogCategory.value === "all" || template.category === catalogCategory.value),
  );
});
const sources = [
  {
    value: "catalog",
    label: "Catálogo",
    detail: "Comece com uma opção pronta.",
  },
  {
    value: "compose",
    label: "Docker Compose",
    detail: "Use uma configuração Compose.",
  },
  {
    value: "image",
    label: "Imagem Docker",
    detail: "Parta de uma imagem existente.",
  },
  { value: "git", label: "Repositório Git", detail: "Conecte um repositório." },
];

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [p, a] = await Promise.all([
      api<Project>(`/api/v1/projects/${route.params.id}`),
      api<Application[]>(`/api/v1/projects/${route.params.id}/applications`),
    ]);
    project.value = p;
    apps.value = a;
  } catch (err) {
    error.value =
      err instanceof Error
        ? err.message
        : "Não foi possível carregar o projeto.";
  } finally {
    loading.value = false;
  }
}
function openEdit() {
  if (!project.value) return;
  editProject.value = {
    name: project.value.name,
    description: project.value.description || "",
  };
  fieldErrors.value = {};
  projectModal.value = true;
}
function openAppEdit(app: Application) {
  editApp.value = { id: app.id, name: app.name };
  fieldErrors.value = {};
  appModal.value = true;
}
function openApp(app: Application) {
  router.push(`/projects/${route.params.id}/applications/${app.id}`);
}
function openCreateApp() {
  editApp.value = { id: 0, name: "" };
  newApp.value = {
    name: "",
    description: "",
    docker_stack_name: "",
    source_type: "",
  };
  newSource.value = {
    compose_yaml: "",
    image: "",
    container_port: null,
    replicas: 1,
    repository_url: "",
    branch: "main",
    dockerfile_path: "Dockerfile",
    build_context: ".",
    command: "",
    entrypoint: "",
    environment: [],
    template_slug: "",
    template_version: "",
  };
  appStep.value = 1;
  fieldErrors.value = {};
  appModal.value = true;
}
function closeWizard() {
  const dirty = Boolean(
    newApp.value.name ||
    newApp.value.description ||
    newApp.value.source_type ||
    newSource.value.compose_yaml ||
    newSource.value.image ||
    newSource.value.repository_url,
  );
  if (dirty) wizardDiscardOpen.value = true;
  else appModal.value = false;
}
function discardWizard() {
  wizardDiscardOpen.value = false;
  appModal.value = false;
}
function selectSourceType(sourceType: string) {
  if (newApp.value.source_type && newApp.value.source_type !== sourceType) {
    pendingSourceType.value = sourceType;
    sourceChangeOpen.value = true;
    return;
  }
  newApp.value.source_type = sourceType;
}
function confirmSourceChange() {
  newApp.value.source_type = pendingSourceType.value;
  newSource.value = {
    ...newSource.value,
    compose_yaml: "",
    image: "",
    container_port: null,
    repository_url: "",
    environment: [],
    template_slug: "",
    template_version: "",
  };
  validationPreview.value = null;
  fieldErrors.value = {};
  sourceChangeOpen.value = false;
}
async function nextAppStep() {
  if (appStep.value === 1 && !newApp.value.name.trim()) {
    fieldErrors.value = { name: "O nome é obrigatório." };
    return;
  }
  if (appStep.value === 2 && !newApp.value.source_type) return;
  if (
    appStep.value === 2 &&
    newApp.value.source_type === "catalog" &&
    !catalogTemplates.value.length
  ) {
    api<typeof catalogTemplates.value>("/api/v1/catalog").then((items) => {
      catalogTemplates.value = items;
    });
  }
  if (appStep.value === 3) {
    validating.value = true;
    try {
      validationPreview.value = await api<typeof validationPreview.value>(
        "/api/v1/source/validate",
        {
          method: "POST",
          body: JSON.stringify({
            source_type: newApp.value.source_type,
            source: newSource.value,
          }),
        },
      );
      if (!validationPreview.value?.valid) {
        fieldErrors.value = {
          source:
            validationPreview.value?.errors?.[0] ||
            "Revise o Compose antes de continuar.",
        };
        return;
      }
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : "Não foi possível validar a origem.",
      );
      return;
    } finally {
      validating.value = false;
    }
  }
  if (
    appStep.value === 3 &&
    newApp.value.source_type === "catalog" &&
    !newSource.value.template_slug
  )
    return;
  appStep.value += 1;
}
async function readComposeFile(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (file) newSource.value.compose_yaml = await file.text();
}
async function saveProject() {
  saving.value = true;
  try {
    await api(`/api/v1/projects/${route.params.id}`, {
      method: "PATCH",
      body: JSON.stringify(editProject.value),
    });
    projectModal.value = false;
    toast.success("Projeto atualizado.");
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
async function createApp(saveAsDraft = false) {
  saving.value = true;
  fieldErrors.value = {};
  try {
    const created = await api<{ id: number }>(
      `/api/v1/projects/${route.params.id}/applications`,
      {
        method: "POST",
        body: JSON.stringify({
          ...newApp.value,
          description: newApp.value.description,
          docker_stack_name: newApp.value.docker_stack_name,
          source: newSource.value,
          save_as_draft: saveAsDraft,
        }),
      },
    );
    appModal.value = false;
    newApp.value = {
      name: "",
      description: "",
      docker_stack_name: "",
      source_type: "",
    };
    appStep.value = 1;
    toast.success("Aplicação adicionada.");
    await load();
    router.push(
      `/projects/${route.params.id}/applications/${created.id}?tab=source`,
    );
  } catch (err) {
    if (err instanceof RequestError) {
      fieldErrors.value = err.fields || {};
      toast.error(err.message);
    }
  } finally {
    saving.value = false;
  }
}
async function submitCreate() {
  await createApp(false);
}
async function saveApp() {
  saving.value = true;
  try {
    await api(`/api/v1/applications/${editApp.value.id}`, {
      method: "PATCH",
      body: JSON.stringify({
        name: editApp.value.name,
      }),
    });
    appModal.value = false;
    toast.success("Aplicação atualizada.");
    await load();
  } catch (err) {
    toast.error(
      err instanceof Error
        ? err.message
        : "Não foi possível atualizar a aplicação.",
    );
  } finally {
    saving.value = false;
  }
}
async function confirmDeleteApp() {
  if (!deleteApp.value) return;
  saving.value = true;
  try {
    await api(`/api/v1/applications/${deleteApp.value.id}`, {
      method: "DELETE",
    });
    toast.success("Aplicação excluída.");
    deleteApp.value = null;
    await load();
  } catch (err) {
    toast.error(
      err instanceof Error
        ? err.message
        : "Não foi possível excluir a aplicação.",
    );
  } finally {
    saving.value = false;
  }
}
onMounted(load);
</script>

<template>
  <div v-if="loading" class="empty">
    <span class="skeleton" />
    <p>Carregando projeto…</p>
  </div>
  <div v-else-if="error" class="empty">
    <h2>Não foi possível carregar</h2>
    <p>{{ error }}</p>
    <button class="secondary" type="button" @click="load">
      Tentar novamente
    </button>
  </div>
  <template v-else-if="project">
    <div class="project-breadcrumb">
      <RouterLink to="/projects">Projetos</RouterLink><span>/</span
      ><strong>{{ project.name }}</strong>
    </div>
    <div class="hero">
      <div>
        <p class="kicker">PROJETO</p>
        <h1>{{ project.name }}</h1>
        <p class="muted">
          {{ project.description || "Sem descrição adicionada." }}
        </p>
      </div>
      <div class="hero-actions">
        <button class="secondary" type="button" @click="openEdit">
          Editar projeto</button
        ><button class="primary" type="button" @click="openCreateApp">
          + Nova aplicação
        </button>
      </div>
    </div>
    <div class="summary-grid">
      <article>
        <span class="label">APLICAÇÕES</span><strong>{{ apps.length }}</strong>
      </article>
      <article>
        <span class="label">STATUS</span
        ><strong class="summary-status">{{
          project.status === "active" ? "Ativo" : project.status
        }}</strong>
      </article>
      <article>
        <span class="label">CRIADO EM</span
        ><strong class="summary-date">{{
          new Date(project.created_at).toLocaleDateString("pt-BR")
        }}</strong>
      </article>
    </div>
    <section class="section-block">
      <div class="section-head">
        <div>
          <p class="kicker">APLICAÇÕES</p>
          <h2>Aplicações deste projeto</h2>
        </div>
      </div>
      <div v-if="!apps.length" class="empty">
        <h2>Nenhuma aplicação ainda</h2>
        <p>
          Crie o registro da primeira aplicação. A publicação será configurada
          em uma próxima etapa.
        </p>
        <button class="secondary" type="button" @click="openCreateApp">
          Adicionar aplicação
        </button>
      </div>
      <div v-else class="app-list">
        <article
          v-for="app in apps"
          :key="app.id"
          class="app-row app-row-clickable"
          role="link"
          tabindex="0"
          @click="openApp(app)"
          @keydown.enter="openApp(app)"
        >
          <div class="app-icon">{{ app.name.charAt(0).toUpperCase() }}</div>
          <div class="app-info">
            <strong>{{ app.name }}</strong
            ><span
              >{{
                sources.find((source) => source.value === app.source_type)
                  ?.label || app.source_type
              }}
              · Publicação ainda não configurada</span
            >
          </div>
          <span class="status-text">{{
            app.status === "unknown" ? "Não publicado" : app.status
          }}</span
          ><button class="ghost" type="button" @click.stop="openAppEdit(app)">
            Editar
          </button>
          <button class="ghost" type="button" @click.stop="deleteApp = app">
            Excluir
          </button>
        </article>
      </div>
    </section>
  </template>

  <BaseModal
    :open="projectModal"
    title="Editar projeto"
    description="Atualize as informações de organização."
    @close="projectModal = false"
  >
    <form id="edit-project-form" @submit.prevent="saveProject">
      <label
        >Nome<input v-model="editProject.name" required autofocus /><small
          v-if="fieldErrors.name"
          class="error-field"
          >{{ fieldErrors.name }}</small
        ></label
      ><label>Descrição<textarea v-model="editProject.description" /></label>
    </form>
    <template #footer
      ><button
        class="secondary"
        type="button"
        :disabled="saving"
        @click="projectModal = false"
      >
        Cancelar</button
      ><button
        class="primary"
        form="edit-project-form"
        type="submit"
        :disabled="saving"
      >
        {{ saving ? "Salvando…" : "Salvar alterações" }}
      </button></template
    >
  </BaseModal>
  <ApplicationWizard
    :open="appModal"
    :editing="Boolean(editApp.id)"
    :step="appStep"
    :busy="saving"
    :validating="validating"
    :source-type="newApp.source_type"
    :can-continue="appStep !== 2 || Boolean(newApp.source_type)"
    @close="closeWizard"
    @back="appStep -= 1"
    @next="nextAppStep"
    @draft="createApp(true)"
    @create="submitCreate"
    @save="saveApp"
  >
    <form v-if="!editApp.id" id="new-app-form" @submit.prevent="submitCreate">
      <p class="kicker">ETAPA {{ appStep }} DE 4</p>
      <div v-if="appStep === 1">
        <h2>Identifique a aplicação</h2>
        <label
          >Nome<input
            v-model="newApp.name"
            required
            autofocus
            placeholder="Ex.: Frontend"
          /><small v-if="fieldErrors.name" class="error-field">{{
            fieldErrors.name
          }}</small></label
        >
        <label
          >Descrição<textarea
            v-model="newApp.description"
            maxlength="2000"
            placeholder="Opcional"
          />
        </label>
        <details class="advanced-options">
          <summary>Opções avançadas</summary>
          <label
            >Nome da stack<input
              v-model="newApp.docker_stack_name"
              placeholder="projeto-aplicacao"
            /><small class="muted"
              >Usado futuramente para identificar a stack no Docker
              Swarm.</small
            ></label
          >
        </details>
        <div class="source-preview identity-preview">
          <span>Identificador</span><strong>{{ slugPreview }}</strong>
        </div>
      </div>
      <div v-else-if="appStep === 2">
        <h2>Escolha a origem</h2>
        <div class="source-grid">
          <button
            v-for="source in sources"
            :key="source.value"
            type="button"
            class="source-option"
            :class="{ selected: newApp.source_type === source.value }"
            @click="selectSourceType(source.value)"
          >
            <strong>{{ source.label }}</strong
            ><small>{{ source.detail }}</small>
          </button>
        </div>
      </div>
      <div v-else-if="appStep === 3">
        <h2>
          Configure
          {{ sources.find((item) => item.value === newApp.source_type)?.label }}
        </h2>
        <div v-if="newApp.source_type === 'compose'">
          <h3>Docker Compose</h3>
          <div class="compose-config-grid">
            <ComposeCodeEditor
              v-model="newSource.compose_yaml"
              @error="toast.error"
            />
            <SourceSummary
              :services="composeSummary.services"
              :images="composeSummary.images"
              :ports="composeSummary.ports"
              :volumes="composeSummary.volumes"
              :networks="composeSummary.networks"
              :empty="!newSource.compose_yaml"
            />
          </div>
          <SourceValidationPanel
            v-if="validationPreview"
            :valid="validationPreview.valid"
            :errors="validationPreview.errors"
            :warnings="validationPreview.warnings"
          />
          <small v-if="fieldErrors.source" class="error-field">{{
            fieldErrors.source
          }}</small>
        </div>
        <div
          v-else-if="newApp.source_type === 'image'"
          class="source-form-stack"
        >
          <h3>Configure a imagem Docker</h3>
          <label
            >Imagem<input
              v-model="newSource.image"
              required
              placeholder="nginx:1.27-alpine" /></label
          ><small class="muted"
            >Imagem que será utilizada para iniciar a aplicação.</small
          ><label
            >Porta interna (opcional)<input
              v-model.number="newSource.container_port"
              type="number"
              min="1"
              placeholder="Ex.: 80"
              max="65535" /></label
          ><label
            >Réplicas
            <div class="stepper-control">
              <button
                type="button"
                aria-label="Diminuir réplicas"
                @click="
                  newSource.replicas = Math.max(1, newSource.replicas - 1)
                "
              >
                −</button
              ><input
                v-model.number="newSource.replicas"
                type="number"
                min="1"
                max="20"
                aria-label="Quantidade de réplicas"
              />
              ><button
                type="button"
                aria-label="Aumentar réplicas"
                @click="
                  newSource.replicas = Math.min(20, newSource.replicas + 1)
                "
              >
                +
              </button>
            </div>
            <small class="muted"
              >Quantidade de instâncias mantidas em execução.</small
            ></label
          >
          <details class="advanced-options">
            <summary>Opções avançadas</summary>
            <label
              >Command<input
                v-model="newSource.command"
                placeholder="Opcional" /></label
            ><label
              >Entrypoint<input
                v-model="newSource.entrypoint"
                placeholder="Opcional"
            /></label>
          </details>
          <EnvironmentVariablesEditor v-model="newSource.environment" />
        </div>
        <div v-else-if="newApp.source_type === 'git'" class="source-form-stack">
          <h3>Configure o repositório Git</h3>
          <label
            >URL do repositório<input
              v-model="newSource.repository_url"
              required
              placeholder="https://github.com/empresa/aplicacao.git"
            /><small class="muted"
              >Aceitamos HTTPS ou SSH. Tokens embutidos na URL não são
              permitidos.</small
            ></label
          >
          <div class="form-grid">
            <label>Branch<input v-model="newSource.branch" /></label
            ><label
              >Dockerfile<input v-model="newSource.dockerfile_path" /><small
                class="muted"
                >Caminho relativo à raiz do repositório.</small
              ></label
            ><label
              >Contexto<input v-model="newSource.build_context" /><small
                class="muted"
                >Diretório enviado ao processo de build.</small
              ></label
            >
          </div>
          <div class="source-preview">
            <span>Repositório</span
            ><strong>{{ newSource.repository_url || "org/repo" }}</strong
            ><span>Build</span
            ><strong>./{{ newSource.dockerfile_path || "Dockerfile" }}</strong
            ><span>Branch</span
            ><strong>{{ newSource.branch || "main" }}</strong>
          </div>
          <EnvironmentVariablesEditor v-model="newSource.environment" />
        </div>
        <div v-else-if="newApp.source_type === 'catalog'" class="source-grid">
          <div class="catalog-toolbar full-width">
            <input v-model="catalogQuery" placeholder="Buscar templates" aria-label="Buscar templates" />
            <select v-model="catalogCategory" aria-label="Filtrar por categoria">
              <option value="all">Todas as categorias</option>
              <option v-for="category in catalogCategories" :key="category" :value="category">{{ category }}</option>
            </select>
          </div>
          <button
            v-for="template in visibleCatalogTemplates"
            :key="template.slug"
            type="button"
            class="source-option"
            :class="{ selected: newSource.template_slug === template.slug }"
            @click="
              newSource.template_slug = template.slug;
              newSource.template_version = template.version;
            "
          >
            <strong>{{ template.name }} · {{ template.version }}</strong
            ><small>{{ template.description }}</small>
          </button>
          <small v-if="!newSource.template_slug" class="error-field"
            >Selecione um template para continuar.</small
          >
          <div v-if="selectedCatalog" class="source-preview full-width">
            <span>Template</span><strong>{{ selectedCatalog.name }}</strong>
            <span>Versão</span><strong>{{ selectedCatalog.version }}</strong>
            <span>Categoria</span
            ><strong>{{ selectedCatalog.category }}</strong>
          </div>
        </div>
        <p v-else class="muted">
          O template oficial Nginx poderá ser selecionado na tela de catálogo.
        </p>
      </div>
      <div v-else class="review-block">
        <h2>Revise antes de criar</h2>
        <section class="review-section">
          <h3>Aplicação</h3>
          <p>
            <strong>{{ newApp.name }}</strong>
          </p>
          <p class="muted">{{ newApp.description || "Sem descrição" }}</p>
          <p class="muted">
            Stack: {{ newApp.docker_stack_name || "projeto-aplicacao" }}
          </p>
        </section>
        <section class="review-section">
          <h3>Origem</h3>
          <p class="muted">
            {{
              sources.find((item) => item.value === newApp.source_type)?.label
            }}
          </p>
          <p v-if="newApp.source_type === 'image'">
            <strong>{{ newSource.image }}</strong> ·
            {{ newSource.replicas }} réplica(s)
          </p>
          <p v-else-if="newApp.source_type === 'git'">
            <strong>{{ newSource.repository_url }}</strong> ·
            {{ newSource.branch }}
          </p>
          <p v-else-if="newApp.source_type === 'catalog'">
            <strong>{{ selectedCatalog?.name }}</strong> ·
            {{ selectedCatalog?.version }}
          </p>
          <p v-else>
            {{ composeSummary.services.length }} serviço(s),
            {{ composeSummary.images.length }} imagem(ns),
            {{ composeSummary.ports.length }} porta(s)
          </p>
        </section>
        <p class="muted">
          Pronta para criar. A configuração será salva com segurança.
        </p>
      </div>
    </form>
    <template #edit><form id="edit-app-form" @submit.prevent="saveApp">
      <label>Nome<input v-model="editApp.name" required autofocus /></label>
      <p class="muted">Edite a configuração da origem na tela da aplicação.</p>
    </form></template>
  </ApplicationWizard>
  <BaseModal
    :open="wizardDiscardOpen"
    title="Descartar configuração?"
    description="Os dados preenchidos nesta aplicação serão perdidos."
    @close="wizardDiscardOpen = false"
  >
    <template #footer>
      <button
        class="secondary"
        type="button"
        @click="wizardDiscardOpen = false"
      >
        Continuar editando
      </button>
      <button class="danger" type="button" @click="discardWizard">
        Descartar
      </button>
    </template>
  </BaseModal>
  <BaseModal
    :open="sourceChangeOpen"
    title="Trocar origem?"
    description="Os dados preenchidos para a origem atual serão removidos."
    @close="sourceChangeOpen = false"
  >
    <template #footer>
      <button class="secondary" type="button" @click="sourceChangeOpen = false">
        Continuar editando
      </button>
      <button class="danger" type="button" @click="confirmSourceChange">
        Trocar origem
      </button>
    </template>
  </BaseModal>
  <BaseModal
    :open="!!deleteApp"
    title="Excluir aplicação"
    :description="`A aplicação ${deleteApp?.name || ''} e sua configuração serão removidas permanentemente.`"
    :busy="saving"
    @close="deleteApp = null"
  >
    <p class="muted">Esta ação não pode ser desfeita.</p>
    <template #footer>
      <button
        class="secondary"
        type="button"
        :disabled="saving"
        @click="deleteApp = null"
      >
        Cancelar
      </button>
      <button
        class="danger"
        type="button"
        :disabled="saving"
        @click="confirmDeleteApp"
      >
        {{ saving ? "Excluindo…" : "Excluir aplicação" }}
      </button>
    </template>
  </BaseModal>
</template>
