<script setup lang="ts">
defineProps<{
  valid: boolean;
  errors?: string[];
  warnings?: string[];
}>();
</script>

<template>
  <aside class="source-validation-panel" :class="{ invalid: !valid, warning: valid && warnings?.length }" aria-live="polite">
    <strong>{{ !valid ? "Revise a configuração" : warnings?.length ? "Válido com avisos" : "Configuração válida" }}</strong>
    <ul v-if="errors?.length">
      <li v-for="message in errors" :key="message">{{ message }}</li>
    </ul>
    <ul v-if="warnings?.length" class="warnings">
      <li v-for="message in warnings" :key="message">{{ message }}</li>
    </ul>
  </aside>
</template>
