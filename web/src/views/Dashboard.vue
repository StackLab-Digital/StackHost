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
const nowGreeting = computed(() => {
  const hour = new Date().getHours();
  return hour < 12 ? "Bom dia" : hour < 18 ? "Boa tarde" : "Boa noite";
});
const infra = computed(() => data.value.infrastructure as Infrastructure);
const userName = ref("Administrador");
async function load() {
  loading.value = true;
  error.value = "";
  try {
    data.value = await api("/api/v1/dashboard");
    try {
      const me = await api<{ name: string }>("/api/v1/me");
      if (me.name) userName.value = me.name.split(" ")[0];
    } catch {
      /* dashboard remains useful if the profile request is unavailable */
    }
  } catch (err) {
    error.value =
      err instanceof Error
        ? err.message
        : "Não foi possível carregar o painel.";
  } finally {
    loading.value = false;
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
  source = new EventSource("/api/v1/events");
  source.onmessage = () => load();
});
onUnmounted(() => source?.close());
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
    <button class="secondary" type="button" @click="load">
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
      <span class="pill">{{
        infra.available
          ? infra.swarm?.active
            ? "Pronto"
            : "Ação necessária"
          : "Atenção"
      }}</span>
    </section>
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
