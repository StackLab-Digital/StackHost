<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api } from "../composables/useApi";
const items = ref<any[]>([]);
const loading = ref(true);
const error = ref("");
const filter = ref("all");
const filters = [
  { id: "all", label: "Todas" },
  { id: "access", label: "Acesso" },
  { id: "projects", label: "Projetos" },
  { id: "applications", label: "Aplicações" },
  { id: "infrastructure", label: "Infraestrutura" },
];
const labels: Record<string, string> = {
  login: "Entrou no painel",
  logout: "Saiu do painel",
  onboarding: "Criou o administrador inicial",
  "project.created": "Criou um projeto",
  "project.updated": "Atualizou um projeto",
  "application.created": "Adicionou uma aplicação",
  "application.updated": "Atualizou uma aplicação",
  "swarm.initialized": "Preparou o ambiente Docker",
};
const visible = computed(() =>
  items.value.filter(
    (item) =>
      filter.value === "all" ||
      (filter.value === "access" &&
        ["login", "logout", "onboarding"].includes(item.action)) ||
      (filter.value === "projects" && item.resource_type === "project") ||
      (filter.value === "applications" &&
        item.resource_type === "application") ||
      (filter.value === "infrastructure" &&
        item.resource_type === "infrastructure"),
  ),
);
async function load() {
  loading.value = true;
  try {
    items.value = await api<any[]>("/api/v1/activity");
  } catch (err) {
    error.value =
      err instanceof Error
        ? err.message
        : "Não foi possível carregar a atividade.";
  } finally {
    loading.value = false;
  }
}
onMounted(load);
</script>
<template>
  <div class="hero">
    <div>
      <p class="kicker">REGISTRO DO AMBIENTE</p>
      <h1>Atividade</h1>
      <p class="muted">
        Um histórico claro das mudanças importantes no painel.
      </p>
    </div>
  </div>
  <div class="tabs filters">
    <button
      v-for="item in filters"
      :key="item.id"
      :class="{ active: filter === item.id }"
      type="button"
      @click="filter = item.id"
    >
      {{ item.label }}
    </button>
  </div>
  <div v-if="loading" class="empty">
    <span class="skeleton" />
    <p>Carregando atividade…</p>
  </div>
  <div v-else-if="error" class="empty">
    <h2>Não foi possível carregar</h2>
    <p>{{ error }}</p>
    <button class="secondary" type="button" @click="load">
      Tentar novamente
    </button>
  </div>
  <div v-else-if="!visible.length" class="empty">
    <h2>Nenhuma atividade encontrada</h2>
    <p>As ações importantes aparecerão aqui.</p>
  </div>
  <div v-else class="activity-list">
    <article v-for="item in visible" :key="item.id">
      <div class="timeline-icon">•</div>
      <div class="activity-copy">
        <strong>{{ labels[item.action] || item.action }}</strong>
        <p class="muted">
          {{ item.actor_name || "Administrador"
          }}<span v-if="item.resource_type"> · {{ item.resource_type }}</span>
        </p>
      </div>
      <time :title="new Date(item.created_at).toLocaleString('pt-BR')">{{
        new Date(item.created_at).toLocaleDateString("pt-BR")
      }}</time>
    </article>
  </div>
</template>
