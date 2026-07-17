<script setup lang="ts">
import { onMounted, ref } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import BaseModal from "../components/ui/BaseModal.vue";
import { api, RequestError } from "../composables/useApi";
import { useToast } from "../composables/useToast";
import type { Application, Project } from "../types";
import { Pencil, Trash2 } from "lucide-vue-next";

const route = useRoute();
const router = useRouter();
const toast = useToast();
const project = ref<Project | null>(null);
const apps = ref<Application[]>([]);
const loading = ref(true);
const error = ref("");
const projectModal = ref(false);
const appModal = ref(false);
const deleteApp = ref<Application | null>(null);
const saving = ref(false);
const fieldErrors = ref<Record<string, string>>({});
const editProject = ref({ name: "", description: "" });
const editApp = ref({ id: 0, name: "" });
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
function applicationStatusLabel(status: string) {
  return {
    configured: "Configurada",
    draft: "Rascunho",
    invalid: "Requer atenção",
    active: "Ativa",
    inactive: "Inativa",
    unknown: "Não publicado",
  }[status] || status;
}

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
  router.push(`/projects/${route.params.id}/applications/new`);
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
          <span class="status-text">{{ applicationStatusLabel(app.status) }}</span
          ><button class="ghost icon-action" type="button" aria-label="Editar aplicação" title="Editar aplicação" @click.stop="openAppEdit(app)">
            <Pencil :size="16" aria-hidden="true" />
          </button>
          <button class="ghost icon-action" type="button" aria-label="Excluir aplicação" title="Excluir aplicação" @click.stop="deleteApp = app">
            <Trash2 :size="16" aria-hidden="true" />
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
    :open="appModal && Boolean(editApp.id)"
    title="Editar aplicação"
    description="Atualize o nome desta aplicação."
    @close="appModal = false"
  >
    <form id="edit-app-form-short" @submit.prevent="saveApp">
      <label>Nome<input v-model="editApp.name" required autofocus /></label>
    </form>
    <template #footer>
      <button class="secondary" type="button" @click="appModal = false">Cancelar</button>
      <button class="primary" form="edit-app-form-short" type="submit" :disabled="saving">{{ saving ? "Salvando…" : "Salvar alterações" }}</button>
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
