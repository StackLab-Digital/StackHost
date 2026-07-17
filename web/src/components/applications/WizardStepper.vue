<script setup lang="ts">
defineProps<{ step: number }>();
const labels = ["Identificação", "Origem", "Configuração", "Revisão"];
</script>
<template>
  <nav class="wizard-stepper" aria-label="Progresso da criação">
    <span
      v-for="(label, index) in labels"
      :key="label"
      :class="{ active: step === index + 1, done: step > index + 1 }"
      :aria-current="step === index + 1 ? 'step' : undefined"
      >{{ index + 1 }}. {{ label }}</span
    >
  </nav>
</template>
