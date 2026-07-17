<script setup lang="ts">
import { ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api, RequestError } from "../composables/useApi";
import { useToast } from "../composables/useToast";

const router = useRouter();
const route = useRoute();
const toast = useToast();
const email = ref("");
const password = ref("");
const error = ref("");
const busy = ref(false);
async function submit() {
  busy.value = true;
  error.value = "";
  try {
    await api("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email: email.value, password: password.value }),
    });
    router.push(
      typeof route.query.redirect === "string" ? route.query.redirect : "/",
    );
  } catch (err) {
    error.value =
      err instanceof RequestError ? err.message : "E-mail ou senha inválidos.";
    toast.error(error.value);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="center">
    <img class="public-logo" src="/brand/stackhost-logo-public.png" alt="StackHost" />
    <p class="kicker">STACKHOST</p>
    <h1>Bem-vindo de volta.</h1>
    <p class="muted">Entre para acessar seu ambiente.</p>
    <form @submit.prevent="submit">
      <label
        >E-mail<input
          v-model="email"
          type="email"
          required
          autofocus
          autocomplete="email" /></label
      ><label
        >Senha<input
          v-model="password"
          type="password"
          required
          autocomplete="current-password"
      /></label>
      <p class="error">{{ error }}</p>
      <button class="primary" type="submit" :disabled="busy">
        {{ busy ? "Entrando…" : "Entrar no painel" }}
      </button>
    </form>
  </div>
</template>
