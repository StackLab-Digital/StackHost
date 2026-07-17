<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { api } from "../composables/useApi";
import { useToast } from "../composables/useToast";
import { useAuthStore } from "../stores/auth";
const auth = useAuthStore();
const router = useRouter();
const toast = useToast();
const infra = ref<any>({});
const loading = ref(true);
const signingOut = ref(false);
const runtimeMode = ref<"standalone" | "swarm">("standalone");
const runtimeSaving = ref(false);
async function load() {
  try {
    infra.value = await api("/api/v1/infrastructure");
    const settings = await api<{ runtime_mode: "standalone" | "swarm" }>("/api/v1/settings/environment");
    runtimeMode.value = settings.runtime_mode;
  } finally {
    loading.value = false;
  }
}
async function saveRuntimeMode() {
  runtimeSaving.value = true;
  try {
    await api("/api/v1/settings/environment", { method: "PATCH", body: JSON.stringify({ runtime_mode: runtimeMode.value }) });
    toast.success("Modo de execução salvo.");
  } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível salvar o modo de execução."); }
  finally { runtimeSaving.value = false; }
}
async function logout() {
  signingOut.value = true;
  try {
    await api("/api/v1/auth/logout", { method: "POST" });
    auth.clear();
    toast.success("Você saiu do painel.");
    router.push("/login");
  } catch (err) {
    toast.error(
      err instanceof Error ? err.message : "Não foi possível sair agora.",
    );
  } finally {
    signingOut.value = false;
  }
}
onMounted(load);
</script>
<template>
  <div class="hero">
    <div>
      <p class="kicker">PREFERÊNCIAS</p>
      <h1>Configurações</h1>
      <p class="muted">Informações da conta e deste ambiente.</p>
    </div>
  </div>
  <div v-if="loading" class="empty">
    <span class="skeleton" />
    <p>Carregando configurações…</p>
  </div>
  <div v-else class="settings-grid">
    <section class="panel panel-section">
      <p class="kicker">CONTA</p>
      <h2>{{ auth.user?.name }}</h2>
      <p class="muted">{{ auth.user?.email }}</p>
      <dl>
        <div>
          <dt>Papel</dt>
          <dd>
            {{ auth.user?.role === "admin" ? "Administrador" : "Usuário" }}
          </dd>
        </div>
        <div>
          <dt>Modo de acesso</dt>
          <dd>Conta local</dd>
        </div>
      </dl>
    </section>
    <section class="panel panel-section runtime-settings">
      <p class="kicker">MODO DE EXECUÇÃO</p>
      <h2>Como as aplicações serão executadas</h2>
      <label><input v-model="runtimeMode" type="radio" value="standalone" /> <strong>Docker em servidor único</strong><small>Mais simples · Executa aplicações com Docker Compose neste servidor.</small></label>
      <label><input v-model="runtimeMode" type="radio" value="swarm" /> <strong>Docker Swarm</strong><small>Alta disponibilidade · Permite múltiplos servidores, réplicas e rolling updates.</small></label>
      <button class="primary" type="button" :disabled="runtimeSaving" @click="saveRuntimeMode">{{ runtimeSaving ? "Salvando…" : "Salvar modo" }}</button>
    </section>
    <section class="panel panel-section">
      <p class="kicker">AMBIENTE</p>
      <h2>Ambiente local</h2>
      <p class="muted">StackHost self-hosted</p>
      <dl>
        <div>
          <dt>Docker</dt>
          <dd>
            {{
              infra.available
                ? `Conectado · ${infra.engine_version || "versão não informada"}`
                : "Indisponível"
            }}
          </dd>
        </div>
        <div>
          <dt>Swarm</dt>
          <dd>
            {{
              infra.swarm?.active ? "Ativo e preparado" : "Ainda não preparado"
            }}
          </dd>
        </div>
      </dl>
    </section>
    <section class="panel panel-section session-panel">
      <p class="kicker">SESSÃO</p>
      <h2>Sessão atual</h2>
      <p class="muted">
        Esta sessão está protegida por cookie seguro e pode ser encerrada a
        qualquer momento.
      </p>
      <button
        class="secondary"
        type="button"
        :disabled="signingOut"
        @click="logout"
      >
        {{ signingOut ? "Saindo…" : "Sair da conta" }}
      </button>
    </section>
  </div>
</template>
