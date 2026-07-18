<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { api } from "../../composables/useApi";
import { useToast } from "../../composables/useToast";

type DeploymentStatus =
  | "queued"
  | "preparing"
  | "deploying"
  | "waiting"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "interrupted";
type Deployment = {
  id: number;
  application_id: number;
  runtime_mode: string;
  status: DeploymentStatus;
  source_revision: number;
  stack_name: string;
  trigger_type: string;
  output: string;
  error_code: string;
  error_message: string;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
};
type DeploymentEvent = {
  event?: string;
  data?: unknown;
};

const props = defineProps<{
  applicationId: number;
  publishing: boolean;
  refreshKey: number;
}>();
const emit = defineEmits<{ publish: [] }>();
const toast = useToast();
const deployments = ref<Deployment[]>([]);
const loading = ref(true);
const error = ref("");
const cancellingId = ref<number | null>(null);
const transientStatuses = new Set<DeploymentStatus>([
  "queued",
  "preparing",
  "deploying",
  "waiting",
]);
const hasTransientDeployments = computed(() =>
  deployments.value.some((deployment) => isTransient(deployment)),
);
const refreshingIds = new Set<number>();
let events: EventSource | undefined;
let pollTimer: number | undefined;
let eventsConnected = false;
let requestId = 0;
let disposed = false;

function isTransient(deployment: Deployment) {
  return transientStatuses.has(deployment.status);
}
function statusLabel(status: DeploymentStatus) {
  return {
    queued: "Na fila",
    preparing: "Preparando",
    deploying: "Publicando",
    waiting: "Verificando execução",
    succeeded: "Concluído",
    failed: "Falhou",
    cancelled: "Cancelado",
    interrupted: "Interrompido",
  }[status];
}
function runtimeModeLabel(mode: string) {
  return mode === "swarm" ? "Docker Swarm" : "Servidor único";
}
function triggerLabel(trigger: string) {
  return (
    {
      manual: "Manual",
      redeploy: "Republicação",
      system: "Sistema",
    }[trigger] || trigger
  );
}
function formatDate(value: string | null) {
  if (!value) return "Aguardando início";
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "Data indisponível"
    : date.toLocaleString("pt-BR");
}
function duration(deployment: Deployment) {
  const start = new Date(deployment.started_at || deployment.created_at).getTime();
  const end = deployment.finished_at
    ? new Date(deployment.finished_at).getTime()
    : Date.now();
  if (!Number.isFinite(start) || !Number.isFinite(end)) return "—";
  const seconds = Math.max(0, Math.floor((end - start) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}min ${seconds % 60}s`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}min`;
}
function message(deployment: Deployment) {
  if (deployment.error_message) return deployment.error_message;
  return {
    queued: "Aguardando o início da publicação.",
    preparing: "Preparando os arquivos e o ambiente.",
    deploying: "Aplicando a revisão no Docker.",
    waiting: "Aguardando o runtime ficar disponível.",
    succeeded: "Publicação concluída com sucesso.",
    failed: "A publicação não pôde ser concluída.",
    cancelled: "Publicação cancelada.",
    interrupted: "A publicação foi interrompida pelo reinício do StackHost.",
  }[deployment.status];
}
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
function sortDeployments(items: Deployment[]) {
  return [...items].sort(
    (left, right) =>
      new Date(right.created_at).getTime() - new Date(left.created_at).getTime(),
  );
}
function upsert(deployment: Deployment) {
  if (disposed || deployment.application_id !== props.applicationId) return;
  const index = deployments.value.findIndex((item) => item.id === deployment.id);
  const next = [...deployments.value];
  if (index === -1) next.push(deployment);
  else next[index] = deployment;
  deployments.value = sortDeployments(next);
  error.value = "";
}
async function loadDeployments(silent = false) {
  const applicationId = props.applicationId;
  const currentRequest = ++requestId;
  if (!silent) loading.value = true;
  try {
    const result = await api<Deployment[]>(
      `/api/v1/applications/${applicationId}/deployments`,
    );
    if (
      disposed ||
      currentRequest !== requestId ||
      applicationId !== props.applicationId
    ) {
      return;
    }
    deployments.value = sortDeployments(Array.isArray(result) ? result : []);
    error.value = "";
  } catch (err) {
    if (
      !disposed &&
      currentRequest === requestId &&
      applicationId === props.applicationId
    ) {
      error.value =
        err instanceof Error
          ? err.message
          : "Não foi possível carregar os deploys.";
    }
  } finally {
    if (!disposed && currentRequest === requestId && !silent) {
      loading.value = false;
    }
  }
}
async function refreshDeployment(deploymentId: number) {
  if (refreshingIds.has(deploymentId)) return;
  const applicationId = props.applicationId;
  refreshingIds.add(deploymentId);
  try {
    const deployment = await api<Deployment>(
      `/api/v1/deployments/${deploymentId}`,
    );
    if (applicationId === props.applicationId) upsert(deployment);
  } catch (err) {
    if (!disposed && applicationId === props.applicationId) {
      error.value =
        err instanceof Error
          ? err.message
          : "Não foi possível atualizar o deploy.";
    }
  } finally {
    refreshingIds.delete(deploymentId);
  }
}
async function pollTransientDeployments() {
  await Promise.all(
    deployments.value
      .filter(isTransient)
      .map((deployment) => refreshDeployment(deployment.id)),
  );
}
function stopPolling() {
  if (pollTimer === undefined) return;
  window.clearInterval(pollTimer);
  pollTimer = undefined;
}
function syncPolling() {
  if (disposed || !hasTransientDeployments.value || eventsConnected) {
    stopPolling();
    return;
  }
  if (pollTimer !== undefined) return;
  pollTimer = window.setInterval(
    () => void pollTransientDeployments(),
    10_000,
  );
}
function handleEvent(event: MessageEvent<string>) {
  let payload: DeploymentEvent;
  try {
    payload = JSON.parse(event.data) as DeploymentEvent;
  } catch {
    return;
  }
  if (!payload.event?.startsWith("deployment.") || !isRecord(payload.data)) {
    return;
  }
  const nestedDeployment = isRecord(payload.data.deployment)
    ? payload.data.deployment
    : undefined;
  const applicationId = Number(
    payload.data.application_id ?? nestedDeployment?.application_id,
  );
  if (applicationId !== props.applicationId) return;
  const deploymentId = Number(
    payload.data.deployment_id ?? payload.data.id ?? nestedDeployment?.id,
  );
  if (Number.isInteger(deploymentId)) void refreshDeployment(deploymentId);
  else void loadDeployments(true);
}
function connectEvents() {
  try {
    events = new EventSource("/api/v1/events");
    events.onmessage = handleEvent;
    events.onopen = () => {
      if (disposed) return;
      eventsConnected = true;
      stopPolling();
    };
    events.onerror = () => {
      if (disposed) return;
      eventsConnected = false;
      syncPolling();
    };
  } catch {
    eventsConnected = false;
    syncPolling();
  }
}
async function cancel(deployment: Deployment) {
  if (!window.confirm("Cancelar esta publicação em andamento?")) return;
  cancellingId.value = deployment.id;
  try {
    const updated = await api<Deployment>(
      `/api/v1/deployments/${deployment.id}/cancel`,
      { method: "POST" },
    );
    upsert(updated);
    toast.info("Cancelamento solicitado. Aguardando confirmação do runtime.");
  } catch (err) {
    toast.error(
      err instanceof Error
        ? err.message
        : "Não foi possível cancelar a publicação.",
    );
  } finally {
    cancellingId.value = null;
  }
}

watch(
  () => props.applicationId,
  () => {
    stopPolling();
    deployments.value = [];
    loading.value = true;
    error.value = "";
    void loadDeployments();
  },
);
watch(
  () => props.refreshKey,
  () => void loadDeployments(true),
);
watch(hasTransientDeployments, syncPolling, { flush: "sync" });
onMounted(() => {
  void loadDeployments();
  connectEvents();
});
onUnmounted(() => {
  disposed = true;
  requestId++;
  events?.close();
  stopPolling();
});
</script>

<template>
  <section
    class="deployments-section"
    aria-live="polite"
    :aria-busy="loading"
  >
    <div class="section-head deployments-head">
      <div>
        <h2>Histórico de deploys</h2>
        <p class="muted">
          Acompanhe cada revisão da fila até o runtime ficar disponível.
        </p>
      </div>
      <button
        class="primary"
        type="button"
        :disabled="props.publishing || hasTransientDeployments"
        @click="emit('publish')"
      >
        {{
          props.publishing
            ? "Publicando…"
            : hasTransientDeployments
              ? "Publicação em andamento"
              : deployments.length
                ? "Publicar novamente"
                : "Publicar aplicação"
        }}
      </button>
    </div>
    <div v-if="loading" class="deployment-loading">
      <div v-for="item in 3" :key="item" class="deployment-skeleton">
        <span class="skeleton" />
        <span class="skeleton short" />
      </div>
    </div>
    <div v-else-if="error && !deployments.length" class="empty deployment-empty">
      <h2>Não foi possível carregar os deploys</h2>
      <p>{{ error }}</p>
      <button class="secondary" type="button" @click="loadDeployments()">
        Tentar novamente
      </button>
    </div>
    <div v-else-if="!deployments.length" class="empty deployment-empty">
      <h2>Nenhuma publicação ainda</h2>
      <p>
        Ao publicar, cada etapa e sua saída técnica aparecerão nesta linha do
        tempo.
      </p>
    </div>
    <template v-else>
      <div v-if="error" class="deployment-inline-error" role="alert">
        <span>{{ error }}</span>
        <button class="ghost" type="button" @click="loadDeployments()">
          Tentar novamente
        </button>
      </div>
      <ol class="deployment-timeline">
        <li v-for="deployment in deployments" :key="deployment.id">
          <div class="deployment-rail" aria-hidden="true">
            <span
              class="deployment-marker"
              :class="`status-${deployment.status}`"
            />
          </div>
          <article class="deployment-entry">
            <div class="deployment-entry-head">
              <div class="deployment-title">
                <span
                  class="deployment-status"
                  :class="`status-${deployment.status}`"
                  >{{ statusLabel(deployment.status) }}</span
                >
                <strong>Revisão {{ deployment.source_revision }}</strong>
              </div>
              <time :datetime="deployment.created_at">
                {{ formatDate(deployment.created_at) }}
              </time>
            </div>
            <p class="deployment-message">{{ message(deployment) }}</p>
            <dl class="deployment-meta-grid">
              <div>
                <dt>Modo</dt>
                <dd>{{ runtimeModeLabel(deployment.runtime_mode) }}</dd>
              </div>
              <div>
                <dt>Início</dt>
                <dd>
                  {{ formatDate(deployment.started_at || deployment.created_at) }}
                </dd>
              </div>
              <div>
                <dt>Duração</dt>
                <dd>{{ duration(deployment) }}</dd>
              </div>
              <div>
                <dt>Origem</dt>
                <dd>{{ triggerLabel(deployment.trigger_type) }}</dd>
              </div>
              <div v-if="deployment.stack_name">
                <dt>Stack</dt>
                <dd>{{ deployment.stack_name }}</dd>
              </div>
            </dl>
            <div v-if="isTransient(deployment)" class="deployment-actions">
              <button
                class="secondary"
                type="button"
                :disabled="cancellingId === deployment.id"
                @click="cancel(deployment)"
              >
                {{ cancellingId === deployment.id ? "Cancelando…" : "Cancelar" }}
              </button>
            </div>
            <details
              v-if="deployment.output || deployment.error_code"
              class="deployment-output"
            >
              <summary>Saída técnica</summary>
              <p v-if="deployment.error_code">
                <strong>Código:</strong> {{ deployment.error_code }}
              </p>
              <pre v-if="deployment.output">{{ deployment.output }}</pre>
            </details>
          </article>
        </li>
      </ol>
    </template>
  </section>
</template>

<style scoped>
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
