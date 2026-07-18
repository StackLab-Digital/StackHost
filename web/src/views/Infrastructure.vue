<script setup lang="ts">
import { onMounted, ref } from "vue";
import BaseModal from "../components/ui/BaseModal.vue";
import { api, RequestError } from "../composables/useApi";
import { useToast } from "../composables/useToast";
import type { Infrastructure } from "../types";
const toast = useToast();
const data = ref<Infrastructure | null>(null);
const nodes = ref<any[]>([]);
const services = ref<any[]>([]);
const inventory = ref<any[]>([]);
const loading = ref(true);
const error = ref("");
const tab = ref("overview");
const prepare = ref(false);
const step = ref(1);
const busy = ref(false);
const advertise = ref("");
const detailError = ref("");
const tabs = [
  { id: "overview", label: "Visão geral" },
  { id: "containers", label: "Containers" },
  { id: "images", label: "Imagens" },
  { id: "volumes", label: "Volumes" },
  { id: "networks", label: "Redes" },
  { id: "swarm", label: "Swarm" },
];
async function load() {
  loading.value = true;
  error.value = "";
  try {
    data.value = await api<Infrastructure>("/api/v1/infrastructure");
    if (data.value.swarm?.active) {
      [nodes.value, services.value] = await Promise.all([
        api<any[]>("/api/v1/infrastructure/nodes"),
        api<any[]>("/api/v1/infrastructure/services"),
      ]);
    }
  } catch (err) {
    error.value =
      err instanceof Error
        ? err.message
        : "Não foi possível carregar a infraestrutura.";
  } finally {
    loading.value = false;
  }
}
async function loadInventory(type: string) {
  if (type === "overview" || type === "swarm") return;
  try {
    inventory.value = await api<any[]>(
      `/api/v1/infrastructure/${type}?limit=100`,
    );
  } catch (err) {
    toast.error(
      err instanceof Error
        ? err.message
        : "Não foi possível carregar o inventário.",
    );
  }
}
async function changeTab(value: string) {
  tab.value = value;
  await loadInventory(value);
}
async function initSwarm() {
  busy.value = true;
  detailError.value = "";
  try {
    await api("/api/v1/infrastructure/swarm/init", {
      method: "POST",
      body: JSON.stringify({ advertise_address: advertise.value }),
    });
    toast.success("Ambiente preparado com sucesso.");
    step.value = 3;
    await load();
  } catch (err) {
    detailError.value =
      err instanceof RequestError
        ? err.message
        : "Não foi possível preparar o ambiente.";
    step.value = 1;
  } finally {
    busy.value = false;
  }
}
function openPrepare() {
  step.value = 1;
  detailError.value = "";
  prepare.value = true;
}
onMounted(load);
</script>
<template>
  <div class="hero">
    <div>
      <p class="kicker">AMBIENTE</p>
      <h1>Infraestrutura</h1>
      <p class="muted">
        Entenda o que está acontecendo no servidor que executa o StackHost.
      </p>
    </div>
    <button
      v-if="data?.available && !data.swarm?.active"
      class="primary"
      type="button"
      @click="openPrepare"
    >
      Preparar ambiente
    </button>
  </div>
  <div v-if="loading" class="empty">
    <span class="skeleton" />
    <p>Consultando o Docker…</p>
  </div>
  <div v-else-if="error" class="empty">
    <h2>Não foi possível consultar o Docker</h2>
    <p>{{ error }}</p>
    <button class="secondary" type="button" @click="load">
      Tentar novamente
    </button>
  </div>
  <template v-else-if="data"
    ><section class="status">
      <span class="dot" :class="{ offline: !data.available }" />
      <div>
        <strong>{{
          data.available ? "Docker conectado" : "Docker indisponível"
        }}</strong>
        <p>
          {{
            data.available
              ? `Engine ${data.engine_version || "sem versão"} · ${data.operating_system || "sistema não informado"}`
              : data.message || "Verifique o Docker Desktop."
          }}
        </p>
      </div>
      <span class="pill">{{
        data.available ? "Disponível" : "Indisponível"
      }}</span>
    </section>
    <div class="tabs" role="tablist">
      <button
        v-for="item in tabs"
        :key="item.id"
        :class="{ active: tab === item.id }"
        type="button"
        role="tab"
        @click="changeTab(item.id)"
      >
        {{ item.label }}
      </button>
    </div>
    <template v-if="tab === 'overview'"
      ><div class="grid metrics">
        <article>
          <span class="label">CONTAINERS</span
          ><strong>{{ data.containers }}</strong>
          <p class="muted">{{ data.running }} em execução</p>
        </article>
        <article>
          <span class="label">IMAGENS</span><strong>{{ data.images }}</strong>
          <p class="muted">Disponíveis localmente</p>
        </article>
        <article>
          <span class="label">VOLUMES</span><strong>{{ data.volumes }}</strong>
          <p class="muted">Dados persistentes</p>
        </article>
        <article>
          <span class="label">REDES</span><strong>{{ data.networks }}</strong>
          <p class="muted">Redes Docker</p>
        </article>
        <article>
          <span class="label">CPU</span
          ><strong>{{ data.cpus || "Não disponível" }}</strong>
          <p class="muted">Núcleos disponíveis</p>
        </article>
        <article>
          <span class="label">MEMÓRIA</span
          ><strong>{{
            data.memory_bytes
              ? `${Math.round(data.memory_bytes / 1073741824)} GB`
              : "Não disponível"
          }}</strong>
          <p class="muted">Capacidade total</p>
        </article>
      </div>
      <section class="panel panel-section">
        <p class="kicker">SWARM</p>
        <h2>
          {{
            data.swarm.active
              ? "Ambiente preparado"
              : "O Swarm ainda não foi iniciado"
          }}
        </h2>
        <p class="muted">
          {{
            data.swarm.active
              ? `${data.swarm.nodes} nós · ${data.swarm.services} serviços · ${data.swarm.tasks_failed} tasks com falha`
              : "Prepare o ambiente para administrar serviços pelo painel."
          }}
        </p>
        <button
          v-if="data.available && !data.swarm.active"
          class="secondary"
          type="button"
          @click="openPrepare"
        >
          Preparar ambiente
        </button>
      </section></template
    ><template v-else-if="tab === 'swarm'"
      ><section class="panel panel-section">
        <h2>
          {{ data.swarm.active ? "Swarm ativo" : "Swarm não inicializado" }}
        </h2>
        <p class="muted">
          {{
            data.swarm.message ||
            "O Swarm permite publicar e atualizar aplicações com segurança."
          }}
        </p>
        <div v-if="data.swarm.active" class="grid">
          <article>
            <span class="label">NÓS</span
            ><strong>{{ data.swarm.nodes }}</strong>
          </article>
          <article>
            <span class="label">MANAGERS</span
            ><strong>{{ data.swarm.managers }}</strong>
          </article>
          <article>
            <span class="label">SERVIÇOS</span
            ><strong>{{ data.swarm.services }}</strong>
          </article>
        </div>
        <button
          v-else-if="data.available"
          class="primary"
          type="button"
          @click="openPrepare"
        >
          Preparar ambiente
        </button>
      </section>
      <div v-if="data.swarm.active && nodes.length" class="data-list">
        <article v-for="node in nodes" :key="node.id">
          <strong>{{ node.hostname }}</strong
          ><span
            >{{ node.role === "manager" ? "Manager" : "Worker" }} ·
            {{ node.availability }} · {{ node.state }}</span
          >
        </article>
      </div></template
    ><template v-else
      ><div v-if="!inventory.length" class="empty">
        <h2>Nenhum item encontrado</h2>
        <p>Não há registros disponíveis nesta categoria.</p>
      </div>
      <div v-else class="data-list">
        <article v-for="item in inventory" :key="item.id || item.name">
          <div>
            <strong>{{ item.name || item.id?.slice(0, 12) }}</strong
            ><span>{{
              item.image || item.driver || item.state || item.tags?.join(", ")
            }}</span>
          </div>
          <span>{{
            item.status ||
            item.scope ||
            (item.size ? `${Math.round(item.size / 1048576)} MB` : "")
          }}</span>
        </article>
      </div></template
    ></template
  ><BaseModal
    :open="prepare"
    title="Preparar ambiente"
    :description="
      step === 1
        ? 'O StackHost usa o Docker Swarm para administrar serviços com segurança.'
        : step === 2
          ? 'Aguarde enquanto o Docker prepara o cluster.'
          : 'O ambiente está pronto para continuar.'
    "
    :close-on-overlay="!busy"
    @close="prepare = false"
    ><div v-if="step === 1" class="prepare-flow">
      <p>
        Este servidor será configurado como o primeiro manager do cluster.
        Containers, imagens e volumes existentes não serão removidos.
      </p>
      <ul>
        <li>Não remove dados existentes</li>
        <li>Habilita o gerenciamento de serviços</li>
        <li>Permite adicionar outros servidores depois</li>
      </ul>
      <details>
        <summary>Opções avançadas</summary>
        <label
          >Endereço de anúncio <span class="muted">(opcional)</span
          ><input
            v-model="advertise"
            placeholder="Deixe vazio para escolha automática"
        /></label>
        <p class="muted">
          Use apenas se o servidor tiver mais de uma interface de rede.
        </p>
      </details>
    </div>
    <div v-else-if="step === 2" class="loading-state">
      <span class="spinner" /><strong>Preparando o ambiente…</strong>
      <p class="muted">Isso pode levar alguns segundos.</p>
    </div>
    <div v-else class="success-state">
      <span class="success-mark">✓</span>
      <h2>Ambiente preparado</h2>
      <p class="muted">
        O servidor agora é um manager do cluster Docker Swarm.
      </p>
    </div>
    <p v-if="detailError" class="error">{{ detailError }}</p>
    <template #footer
      ><button
        v-if="step === 1"
        class="secondary"
        type="button"
        @click="prepare = false"
      >
        Agora não</button
      ><button
        v-if="step === 1"
        class="primary"
        type="button"
        :disabled="busy"
        @click="
          step = 2;
          initSwarm();
        "
      >
        Preparar agora</button
      ><button
        v-if="step === 3"
        class="primary"
        type="button"
        @click="prepare = false"
      >
        Ver infraestrutura</button
      ><button
        v-if="detailError"
        class="primary"
        type="button"
        :disabled="busy"
        @click="
          step = 2;
          initSwarm();
        "
      >
        Tentar novamente
      </button></template
    ></BaseModal
  >
</template>
