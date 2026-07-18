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
const backups = ref<Array<{ id: number; status: string; size_bytes: number; created_at: string }>>([]);
const backupBusy = ref(false);
const backupSchedule = ref<"manual" | "daily" | "weekly">("manual");
const backupRetention = ref(7);
const backupSettingsBusy = ref(false);
const notifications = ref<Array<{ id: number; name: string; kind: string; last_status: string }>>([]);
const notificationName = ref("");
const notificationURL = ref("");
const notificationBusy = ref(false);
async function load() {
  try {
    infra.value = await api("/api/v1/infrastructure");
    const settings = await api<{ runtime_mode: "standalone" | "swarm" }>("/api/v1/settings/environment");
    runtimeMode.value = settings.runtime_mode;
    if (auth.user?.role === "admin") backups.value = await api<typeof backups.value>("/api/v1/backups/system");
    if (auth.user?.role === "admin") {
      const settings = await api<{ schedule: "manual" | "daily" | "weekly"; retention: number }>("/api/v1/backups/settings");
      backupSchedule.value = settings.schedule; backupRetention.value = settings.retention;
    }
    if (auth.user?.role === "admin") notifications.value = await api<typeof notifications.value>("/api/v1/notifications");
  } finally {
    loading.value = false;
  }
}
async function createNotification() {
  notificationBusy.value = true;
  try {
    await api("/api/v1/notifications", { method: "POST", body: JSON.stringify({ name: notificationName.value, kind: "webhook", url: notificationURL.value, events: ["deployment.succeeded", "deployment.failed"] }) });
    notifications.value = await api<typeof notifications.value>("/api/v1/notifications"); notificationName.value = ""; notificationURL.value = ""; toast.success("Notificação salva.");
  } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível salvar a notificação."); }
  finally { notificationBusy.value = false; }
}
async function testNotification(id: number) {
  try { await api(`/api/v1/notifications/${id}/test`, { method: "POST" }); toast.success("Teste enviado."); }
  catch (err) { toast.error(err instanceof Error ? err.message : "O teste falhou."); }
}
async function createBackup() {
  backupBusy.value = true;
  try { await api("/api/v1/backups/system", { method: "POST" }); backups.value = await api<typeof backups.value>("/api/v1/backups/system"); toast.success("Backup criado."); }
  catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível criar o backup."); }
  finally { backupBusy.value = false; }
}
async function saveBackupSettings() {
  backupSettingsBusy.value = true;
  try { await api("/api/v1/backups/settings", { method: "PATCH", body: JSON.stringify({ schedule: backupSchedule.value, retention: backupRetention.value }) }); toast.success("Agenda de backup salva."); }
  catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível salvar a agenda."); }
  finally { backupSettingsBusy.value = false; }
}
async function removeBackup(id: number) {
  try { await api(`/api/v1/backups/system/${id}`, { method: "DELETE" }); backups.value = backups.value.filter((item) => item.id !== id); }
  catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível remover o backup."); }
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
    <section v-if="auth.user?.role === 'admin'" class="panel panel-section">
      <p class="kicker">BACKUPS</p>
      <h2>Recuperação do StackHost</h2>
      <p class="muted">Inclui o SQLite e os certificados locais. Os arquivos ficam somente neste servidor.</p>
      <button class="secondary" type="button" :disabled="backupBusy" @click="createBackup">{{ backupBusy ? "Criando…" : "Criar backup agora" }}</button>
      <div class="settings-form"><label>Agenda <select v-model="backupSchedule"><option value="manual">Manual</option><option value="daily">Diária</option><option value="weekly">Semanal</option></select></label><label>Reter <input v-model.number="backupRetention" type="number" min="1" max="100" /> backups</label><button class="ghost" type="button" :disabled="backupSettingsBusy" @click="saveBackupSettings">Salvar agenda</button></div>
      <ul v-if="backups.length" class="settings-list">
        <li v-for="backup in backups" :key="backup.id"><span>{{ new Date(backup.created_at).toLocaleString() }}</span><a :href="`/api/v1/backups/system/${backup.id}/download`">Baixar</a><button class="ghost" type="button" @click="removeBackup(backup.id)">Remover</button></li>
      </ul>
    </section>
    <section v-if="auth.user?.role === 'admin'" class="panel panel-section">
      <p class="kicker">NOTIFICAÇÕES</p>
      <h2>Webhooks operacionais</h2>
      <p class="muted">URLs são protegidas no banco. Comece com deploy concluído ou falho.</p>
      <div class="settings-form"><input v-model="notificationName" aria-label="Nome da notificação" placeholder="Nome" /><input v-model="notificationURL" aria-label="URL do webhook" type="url" placeholder="https://…" /><button class="secondary" type="button" :disabled="notificationBusy" @click="createNotification">Adicionar webhook</button></div>
      <ul v-if="notifications.length" class="settings-list"><li v-for="item in notifications" :key="item.id"><span>{{ item.name }} · {{ item.kind }}</span><button class="ghost" type="button" @click="testNotification(item.id)">Testar</button></li></ul>
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
