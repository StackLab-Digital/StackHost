<script setup lang="ts">
import { onMounted, ref } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import BaseModal from "../components/ui/BaseModal.vue";
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
const saving = ref(false);
const fieldErrors = ref<Record<string, string>>({});
const editProject = ref({ name: "", description: "" });
const editApp = ref({ id: 0, name: "" });
const newApp = ref({ name: "", source_type: "" });
const appStep = ref(1);
const newSource = ref({
  compose_yaml: "",
  image: "",
  container_port: 0,
  replicas: 1,
  repository_url: "",
  branch: "main",
  dockerfile_path: "Dockerfile",
  build_context: ".",
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
  newApp.value = { name: "", source_type: "" };
  newSource.value = {
    compose_yaml: "",
    image: "",
    container_port: 0,
    replicas: 1,
    repository_url: "",
    branch: "main",
    dockerfile_path: "Dockerfile",
    build_context: ".",
  };
  appStep.value = 1;
  fieldErrors.value = {};
  appModal.value = true;
}
function nextAppStep() {
  if (appStep.value === 1 && !newApp.value.name.trim()) {
    fieldErrors.value = { name: "O nome é obrigatório." };
    return;
  }
  if (appStep.value === 2 && !newApp.value.source_type) return;
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
async function createApp() {
  saving.value = true;
  fieldErrors.value = {};
  try {
    await api(`/api/v1/projects/${route.params.id}/applications`, {
      method: "POST",
      body: JSON.stringify({
        ...newApp.value,
        description: "",
        source: newSource.value,
        save_as_draft: true,
      }),
    });
    appModal.value = false;
    newApp.value = { name: "", source_type: "" };
    appStep.value = 1;
    toast.success("Aplicação adicionada.");
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
  <BaseModal
    :open="appModal"
    :title="editApp.id ? 'Editar aplicação' : 'Nova aplicação'"
    description="A aplicação será criada agora; a publicação será configurada depois."
    @close="appModal = false"
  >
    <form v-if="!editApp.id" id="new-app-form" @submit.prevent="createApp">
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
        <p class="muted">
          Slug:
          {{
            newApp.name.toLowerCase().trim().replace(/\s+/g, "-") ||
            "nome-da-aplicacao"
          }}
        </p>
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
            @click="newApp.source_type = source.value"
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
        <label v-if="newApp.source_type === 'compose'"
          >Docker Compose<textarea
            v-model="newSource.compose_yaml"
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
        <div v-else-if="newApp.source_type === 'image'" class="form-grid">
          <label
            >Imagem Docker<input
              v-model="newSource.image"
              required
              placeholder="nginx:1.27-alpine" /></label
          ><label
            >Porta interna<input
              v-model.number="newSource.container_port"
              type="number"
              min="0"
              max="65535" /></label
          ><label
            >Réplicas<input
              v-model.number="newSource.replicas"
              type="number"
              min="1"
          /></label>
        </div>
        <div v-else-if="newApp.source_type === 'git'" class="form-grid">
          <label
            >URL do repositório<input
              v-model="newSource.repository_url"
              required
              placeholder="https://github.com/org/repo.git" /></label
          ><label>Branch<input v-model="newSource.branch" /></label
          ><label>Dockerfile<input v-model="newSource.dockerfile_path" /></label
          ><label>Contexto<input v-model="newSource.build_context" /></label>
        </div>
        <p v-else class="muted">
          O template oficial Nginx poderá ser selecionado na tela de catálogo.
        </p>
      </div>
      <div v-else class="review-block">
        <h2>Revise antes de criar</h2>
        <p>
          <strong>{{ newApp.name }}</strong>
        </p>
        <p class="muted">
          {{ sources.find((item) => item.value === newApp.source_type)?.label }}
          · A publicação será configurada em uma próxima etapa.
        </p>
        <p class="muted">
          A configuração será salva com segurança e poderá ser validada depois.
        </p>
      </div>
    </form>
    <form v-else id="edit-app-form" @submit.prevent="saveApp">
      <label>Nome<input v-model="editApp.name" required autofocus /></label>
      <p class="muted">Edite a configuração da origem na tela da aplicação.</p>
    </form>
    <template #footer
      ><button
        class="secondary"
        type="button"
        :disabled="saving"
        @click="appModal = false"
      >
        Cancelar</button
      ><button
        v-if="!editApp.id && appStep > 1"
        class="secondary"
        type="button"
        :disabled="saving"
        @click="appStep -= 1"
      >
        Voltar</button
      ><button
        v-if="!editApp.id && appStep < 4"
        class="primary"
        type="button"
        :disabled="saving || (appStep === 2 && !newApp.source_type)"
        @click="nextAppStep"
      >
        Continuar</button
      ><button
        v-if="!editApp.id && appStep === 4"
        class="primary"
        form="new-app-form"
        type="submit"
        :disabled="saving"
      >
        {{ saving ? "Criando…" : "Criar aplicação" }}</button
      ><button
        v-else
        class="primary"
        form="edit-app-form"
        type="submit"
        :disabled="saving"
      >
        {{ saving ? "Salvando…" : "Salvar alterações" }}
      </button></template
    >
  </BaseModal>
</template>
