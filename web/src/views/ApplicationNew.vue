<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { onBeforeRouteLeave, useRoute, useRouter } from "vue-router";
import ApplicationWizard from "../components/applications/ApplicationWizard.vue";
import ComposeCodeEditor from "../components/applications/ComposeCodeEditor.vue";
import EnvironmentVariablesEditor from "../components/applications/EnvironmentVariablesEditor.vue";
import SourceValidationPanel from "../components/applications/SourceValidationPanel.vue";
import { api, RequestError } from "../composables/useApi";
import { useToast } from "../composables/useToast";

const route = useRoute();
const router = useRouter();
const toast = useToast();
const step = ref(1);
const saving = ref(false);
const validating = ref(false);
const discardOpen = ref(false);
const allowNavigation = ref(false);
const fieldErrors = ref<Record<string, string>>({});
type SourceValidation = { valid: boolean; errors: string[]; warnings: string[]; summary?: { services?: string[]; images?: string[]; ports?: string[]; volumes?: string[]; networks?: string[]; secrets?: string[]; configs?: string[] } };
const validation = ref<SourceValidation | null>(null);
const newApp = ref({ name: "", description: "", docker_stack_name: "", source_type: "" });
const source = ref({ compose_yaml: "", image: "", container_port: null as number | null, replicas: 1, repository_url: "", branch: "main", dockerfile_path: "Dockerfile", build_context: ".", command: "", entrypoint: "", environment: [] as Array<{ key: string; value: string; secret: boolean }>, template_slug: "", template_version: "" });
const catalog = ref<Array<{ slug: string; name: string; description: string; version: string; category: string; source: string }>>([]);
const query = ref("");
const category = ref("all");
const sourceTypes = [
  { value: "catalog", label: "Catálogo", detail: "Comece com uma opção pronta." },
  { value: "compose", label: "Docker Compose", detail: "Use uma configuração Compose." },
  { value: "image", label: "Imagem Docker", detail: "Parta de uma imagem existente." },
  { value: "git", label: "Repositório Git", detail: "Conecte um repositório." },
];
const slug = computed(() => newApp.value.name.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 100) || "nome-da-aplicacao");
const categories = computed(() => [...new Set(catalog.value.map((item) => item.category))]);
const visibleCatalog = computed(() => catalog.value.filter((item) => (!query.value || `${item.name} ${item.description}`.toLowerCase().includes(query.value.toLowerCase())) && (category.value === "all" || item.category === category.value)));
const selectedTemplate = computed(() => catalog.value.find((item) => item.slug === source.value.template_slug));
const summary = computed(() => ({
  services: validation.value?.summary?.services || [], images: validation.value?.summary?.images || [], ports: validation.value?.summary?.ports || [],
  volumes: validation.value?.summary?.volumes || [], networks: validation.value?.summary?.networks || [],
}));
watch(() => source.value.compose_yaml, () => { if (validation.value) validation.value = null; });
const dirty = computed(() => Boolean(newApp.value.name || newApp.value.description || newApp.value.source_type || source.value.compose_yaml || source.value.image || source.value.repository_url));
function goBack() {
  if (dirty.value) discardOpen.value = true;
  else router.push(`/projects/${route.params.projectId}`);
}
function discard() { discardOpen.value = false; router.push(`/projects/${route.params.projectId}`); }
function selectSource(value: string) { newApp.value.source_type = value; }
async function next() {
  if (step.value === 1 && !newApp.value.name.trim()) { fieldErrors.value = { name: "O nome é obrigatório." }; return; }
  if (step.value === 2 && newApp.value.source_type === "catalog" && !catalog.value.length) catalog.value = await api<typeof catalog.value>("/api/v1/catalog");
  if (step.value === 3) {
    validating.value = true;
    try {
      validation.value = await api<typeof validation.value>("/api/v1/source/validate", { method: "POST", body: JSON.stringify({ source_type: newApp.value.source_type, source: source.value }) });
      if (!validation.value?.valid) { fieldErrors.value = { source: validation.value?.errors?.[0] || "Revise a configuração." }; return; }
    } catch (error) { toast.error(error instanceof Error ? error.message : "Não foi possível validar a origem."); return; }
    finally { validating.value = false; }
  }
  if (step.value === 3 && newApp.value.source_type === "catalog" && !source.value.template_slug) return;
  step.value += 1;
}
async function create(save_as_draft = false) {
  saving.value = true;
  try {
    const result = await api<{ id: number }>(`/api/v1/projects/${route.params.projectId}/applications`, { method: "POST", body: JSON.stringify({ ...newApp.value, source: source.value, save_as_draft }) });
    toast.success("Aplicação adicionada.");
    allowNavigation.value = true;
    router.push(`/projects/${route.params.projectId}/applications/${result.id}?tab=source`);
  } catch (error) {
    if (error instanceof RequestError) fieldErrors.value = error.fields || {};
    toast.error(error instanceof Error ? error.message : "Não foi possível criar a aplicação.");
  } finally { saving.value = false; }
}
onMounted(() => { document.title = "Nova aplicação · StackHost"; });
onBeforeRouteLeave((_to, _from, next) => {
  if (allowNavigation.value) {
    next();
    return;
  }
  if (dirty.value && !discardOpen.value) {
    discardOpen.value = true;
    next(false);
    return;
  }
  next();
});
</script>

<template>
  <ApplicationWizard :step="step" :busy="saving" :validating="validating" :source-type="newApp.source_type" :can-continue="step !== 2 || Boolean(newApp.source_type)" @close="goBack" @back="step -= 1" @next="next" @draft="create(true)" @create="create()">
    <div class="wizard-step-content" :class="{ 'wizard-step-content--compose': step === 3 && newApp.source_type === 'compose' }">
      <p v-if="!(step === 3 && newApp.source_type === 'compose')" class="kicker">ETAPA {{ step }} DE 4</p>
      <section v-if="step === 1">
        <h2>Identifique a aplicação</h2>
        <label>Nome<input v-model="newApp.name" autofocus required placeholder="Ex.: Frontend" /><small v-if="fieldErrors.name" class="error-field">{{ fieldErrors.name }}</small></label>
        <label>Descrição<textarea v-model="newApp.description" maxlength="2000" placeholder="Opcional" /></label>
        <details class="advanced-options"><summary>Opções avançadas</summary><label>Nome da stack<input v-model="newApp.docker_stack_name" placeholder="projeto-aplicacao" /><small class="muted">Usado futuramente para identificar a stack no Docker Swarm.</small></label></details>
        <div class="source-preview identity-preview"><span>Identificador</span><strong>{{ slug }}</strong></div>
      </section>
      <section v-else-if="step === 2"><h2>Como esta aplicação será publicada?</h2><div class="source-grid"><button v-for="item in sourceTypes" :key="item.value" type="button" class="source-option" :class="{ selected: newApp.source_type === item.value }" @click="selectSource(item.value)"><strong>{{ item.label }}</strong><small>{{ item.detail }}</small></button></div></section>
      <section v-else-if="step === 3">
        <div v-if="newApp.source_type === 'compose'" class="compose-step"><div class="compose-workspace"><ComposeCodeEditor v-model="source.compose_yaml" fill :validation="validation ? (validation.valid ? (validation.warnings.length ? 'warning' : 'valid') : 'invalid') : ''" @error="toast.error" /><SourceValidationPanel v-if="validation && (!validation.valid || validation.warnings.length)" :valid="validation.valid" :errors="validation.errors" :warnings="validation.warnings" /></div><small v-if="fieldErrors.source" class="error-field">{{ fieldErrors.source }}</small></div>
        <div v-else-if="newApp.source_type === 'image'" class="source-form-stack"><h3>Configure a imagem Docker</h3><label>Imagem<input v-model="source.image" placeholder="nginx:1.27-alpine" /></label><small class="muted">Imagem que será utilizada para iniciar a aplicação.</small><label>Porta interna (opcional)<input v-model.number="source.container_port" type="number" min="1" max="65535" placeholder="Ex.: 80" /></label><label>Réplicas<div class="stepper-control"><button type="button" aria-label="Diminuir réplicas" @click="source.replicas = Math.max(1, source.replicas - 1)">−</button><input v-model.number="source.replicas" type="number" min="1" max="20" aria-label="Quantidade de réplicas" /><button type="button" aria-label="Aumentar réplicas" @click="source.replicas = Math.min(20, source.replicas + 1)">+</button></div><small class="muted">Quantidade de instâncias mantidas em execução.</small></label><EnvironmentVariablesEditor v-model="source.environment" /></div>
        <div v-else-if="newApp.source_type === 'git'" class="source-form-stack"><h3>Configure o repositório Git</h3><label>URL do repositório<input v-model="source.repository_url" placeholder="https://github.com/empresa/aplicacao.git" /><small class="muted">Aceitamos HTTPS ou SSH. Tokens embutidos na URL não são permitidos.</small></label><div class="form-grid"><label>Branch<input v-model="source.branch" /></label><label>Dockerfile<input v-model="source.dockerfile_path" /><small class="muted">Caminho relativo à raiz do repositório.</small></label><label>Contexto<input v-model="source.build_context" /><small class="muted">Diretório enviado ao processo de build.</small></label></div><EnvironmentVariablesEditor v-model="source.environment" /></div>
        <div v-else-if="newApp.source_type === 'catalog'" class="source-grid"><div class="catalog-toolbar full-width"><input v-model="query" aria-label="Buscar templates" placeholder="Buscar templates" /><select v-model="category" aria-label="Filtrar por categoria"><option value="all">Todas as categorias</option><option v-for="item in categories" :key="item" :value="item">{{ item }}</option></select></div><button v-for="item in visibleCatalog" :key="item.slug" type="button" class="source-option" :class="{ selected: source.template_slug === item.slug }" @click="source.template_slug = item.slug; source.template_version = item.version"><strong>{{ item.name }} · {{ item.version }}</strong><small>{{ item.description }}</small></button><small v-if="!source.template_slug" class="error-field">Selecione um template para continuar.</small><div v-if="selectedTemplate" class="source-preview full-width"><span>Template</span><strong>{{ selectedTemplate.name }}</strong><span>Versão</span><strong>{{ selectedTemplate.version }}</strong></div></div>
      </section>
      <section v-else class="review-block"><h2>Revise antes de criar</h2><div class="review-section"><h3>Aplicação</h3><p><strong>{{ newApp.name }}</strong></p><p class="muted">{{ newApp.description || "Sem descrição" }}</p><p class="muted">Stack: {{ newApp.docker_stack_name || "projeto-aplicacao" }}</p></div><div class="review-section"><h3>Origem</h3><p class="muted">{{ sourceTypes.find((item) => item.value === newApp.source_type)?.label }}</p><p v-if="newApp.source_type === 'compose'">{{ summary.services.length }} serviço(s), {{ summary.images.length }} imagem(ns), {{ summary.ports.length }} porta(s)</p><p v-else-if="newApp.source_type === 'image'"><strong>{{ source.image }}</strong> · {{ source.replicas }} réplica(s)</p><p v-else-if="newApp.source_type === 'git'"><strong>{{ source.repository_url }}</strong> · {{ source.branch }}</p><p v-else><strong>{{ selectedTemplate?.name }}</strong> · {{ selectedTemplate?.version }}</p></div><p class="muted">Pronta para criar.</p></section>
    </div>
  </ApplicationWizard>
  <div v-if="discardOpen" class="wizard-confirm" role="alertdialog" aria-live="assertive">
    <strong>Descartar configuração?</strong>
    <p>Os dados preenchidos nesta aplicação serão perdidos.</p>
    <div><button class="secondary" type="button" @click="discardOpen = false">Continuar editando</button><button class="danger" type="button" @click="discard">Descartar</button></div>
  </div>
</template>
