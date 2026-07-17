<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import BaseModal from "../components/ui/BaseModal.vue";
import { api, RequestError } from "../composables/useApi";
import { useToast } from "../composables/useToast";
import type { Project } from "../types";
const projects = ref<Project[]>([]);
const loading = ref(true);
const error = ref("");
const query = ref("");
const open = ref(false);
const saving = ref(false);
const fields = ref({ name: "", description: "" });
const fieldErrors = ref<Record<string, string>>({});
const toast = useToast();
const filtered = computed(() =>
  projects.value.filter((project) =>
    `${project.name} ${project.slug} ${project.description}`
      .toLowerCase()
      .includes(query.value.toLowerCase()),
  ),
);
async function load() {
  loading.value = true;
  try {
    projects.value = await api<Project[]>("/api/v1/projects");
  } catch (err) {
    error.value =
      err instanceof Error
        ? err.message
        : "Não foi possível carregar os projetos.";
  } finally {
    loading.value = false;
  }
}
async function create() {
  saving.value = true;
  fieldErrors.value = {};
  try {
    await api("/api/v1/projects", {
      method: "POST",
      body: JSON.stringify(fields.value),
    });
    open.value = false;
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
onMounted(load);
</script>
<template>
  <div class="hero">
    <div>
      <p class="kicker">SEUS AMBIENTES</p>
      <h1>Projetos</h1>
      <p class="muted">Mantenha suas aplicações organizadas.</p>
    </div>
    <button class="primary" type="button" @click="open = true">
      + Novo projeto
    </button>
  </div>
  <div class="toolbar">
    <input
      v-model="query"
      aria-label="Buscar projetos"
      placeholder="Buscar projetos…"
    />
  </div>
  <div v-if="loading" class="grid">
    <article v-for="n in 3" :key="n"><span class="skeleton" /></article>
  </div>
  <div v-else-if="error" class="empty">
    <h2>Não foi possível carregar</h2>
    <p>{{ error }}</p>
    <button class="secondary" type="button" @click="load">
      Tentar novamente
    </button>
  </div>
  <div v-else-if="!filtered.length" class="empty">
    <h2>
      {{
        query ? "Nenhum projeto encontrado" : "Comece pelo seu primeiro projeto"
      }}
    </h2>
    <p>
      {{
        query
          ? "Tente buscar por outro nome ou slug."
          : "Um projeto é o ponto de partida para organizar suas aplicações."
      }}
    </p>
    <button v-if="!query" class="secondary" type="button" @click="open = true">
      Criar primeiro projeto
    </button>
  </div>
  <div v-else class="cards-grid">
    <RouterLink
      v-for="project in filtered"
      :key="project.id"
      class="project-card"
      :to="`/projects/${project.id}`"
      ><div class="card-top">
        <span class="status-mark"><span class="dot" /> Ativo</span
        ><span class="arrow">↗</span>
      </div>
      <h2>{{ project.name }}</h2>
      <p class="muted">{{ project.description || project.slug }}</p>
      <div class="card-meta">
        <span>{{ project.applications_count || 0 }} aplicações</span
        ><span
          >Atualizado
          {{ new Date(project.updated_at).toLocaleDateString("pt-BR") }}</span
        >
      </div></RouterLink
    >
  </div>
  <BaseModal
    :open="open"
    title="Novo projeto"
    description="Dê um nome e uma breve descrição para começar."
    @close="open = false"
    ><form id="projects-form" @submit.prevent="create">
      <label
        >Nome<input
          v-model="fields.name"
          required
          autofocus
          placeholder="Ex.: Loja online"
        /><small v-if="fieldErrors.name" class="error-field">{{
          fieldErrors.name
        }}</small></label
      ><label
        >Descrição <span class="muted">(opcional)</span
        ><textarea
          v-model="fields.description"
          placeholder="Uma descrição curta"
        ></textarea>
      </label>
    </form>
    <template #footer
      ><button
        class="secondary"
        type="button"
        :disabled="saving"
        @click="open = false"
      >
        Cancelar</button
      ><button
        class="primary"
        form="projects-form"
        type="submit"
        :disabled="saving"
      >
        {{ saving ? "Criando…" : "Criar projeto" }}
      </button></template
    ></BaseModal
  >
</template>
