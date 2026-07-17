<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { api, RequestError } from "../composables/useApi";
import { useToast } from "../composables/useToast";

const router = useRouter();
const toast = useToast();
const step = ref(1);
const name = ref("");
const email = ref("");
const password = ref("");
const error = ref("");
const busy = ref(false);
const environment = ref<any>({});
const fields = ref<Record<string, string>>({});
onMounted(async () => {
  environment.value =
    (await api<any>("/api/v1/setup/status")).infrastructure || {};
});
async function submit() {
  busy.value = true;
  error.value = "";
  fields.value = {};
  try {
    await api("/api/v1/setup/admin", {
      method: "POST",
      body: JSON.stringify({
        name: name.value,
        email: email.value,
        password: password.value,
      }),
    });
    step.value = 4;
    toast.success("Administrador criado com sucesso.");
  } catch (err) {
    if (err instanceof RequestError) {
      fields.value = err.fields || {};
      error.value = err.message;
    }
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="center">
    <img class="public-logo" src="/brand/stackhost-logo-public.png" alt="StackHost" />
    <p class="kicker">ETAPA {{ step }} DE 4 · CONFIGURAÇÃO</p>
    <div v-if="step === 1">
      <h1>Seu ambiente, sob controle.</h1>
      <p class="muted">
        Uma forma simples e segura de cuidar da infraestrutura.
      </p>
      <div class="empty">
        <h2>Bem-vindo ao StackHost</h2>
        <p>
          Vamos criar seu administrador e verificar o ambiente em poucos
          segundos.
        </p>
        <button class="primary" type="button" @click="step = 2">Começar</button>
      </div>
    </div>
    <div v-else-if="step === 2">
      <h1>Crie seu administrador.</h1>
      <p class="muted">Esta conta gerencia este ambiente local.</p>
      <form @submit.prevent="step = 3">
        <label
          >Nome<input
            v-model="name"
            required
            autofocus
            autocomplete="name"
          /><small v-if="fields.name" class="error-field">{{
            fields.name
          }}</small></label
        ><label
          >E-mail<input
            v-model="email"
            type="email"
            required
            autocomplete="email"
          /><small v-if="fields.email" class="error-field">{{
            fields.email
          }}</small></label
        ><label
          >Senha<input
            v-model="password"
            type="password"
            minlength="8"
            required
            autocomplete="new-password"
          /><small v-if="fields.password" class="error-field">{{
            fields.password
          }}</small></label
        ><button class="primary" type="submit">Continuar</button>
      </form>
    </div>
    <div v-else-if="step === 3">
      <h1>Verifique o ambiente.</h1>
      <p class="muted">
        O Docker é opcional para criar sua conta e pode ser conectado depois.
      </p>
      <div class="status">
        <span class="dot" :class="{ offline: !environment.available }" />
        <div>
          <strong>{{
            environment.available ? "Docker conectado" : "Docker opcional"
          }}</strong>
          <p>
            {{
              environment.available
                ? `Docker ${environment.engine_version || ""} · ${environment.operating_system || "sistema não informado"} · ${environment.cpus || "Não disponível"} CPUs`
                : "O painel pode continuar sem Docker."
            }}
          </p>
        </div>
        <span class="pill">{{
          environment.swarm?.active ? "Swarm pronto" : "Sem Swarm"
        }}</span>
      </div>
      <button class="primary" type="button" :disabled="busy" @click="submit">
        {{ busy ? "Criando…" : "Criar administrador" }}
      </button>
    </div>
    <div v-else>
      <h1>Tudo pronto.</h1>
      <p class="muted">Seu administrador está ativo.</p>
      <div class="empty">
        <h2>Bem-vindo ao StackHost</h2>
        <p>Agora você pode começar a organizar seus projetos.</p>
        <button class="primary" type="button" @click="router.push('/')">
          Abrir painel
        </button>
      </div>
    </div>
    <p class="error">{{ error }}</p>
  </div>
</template>
