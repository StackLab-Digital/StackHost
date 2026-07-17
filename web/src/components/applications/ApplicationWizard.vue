<script setup lang="ts">
import WizardStepper from "./WizardStepper.vue";

defineProps<{
  step: number;
  busy?: boolean;
  validating?: boolean;
  sourceType?: string;
  canContinue?: boolean;
}>();
const emit = defineEmits<{
  close: [];
  back: [];
  next: [];
  draft: [];
  create: [];
}>();
</script>

<template>
  <main class="application-wizard-page">
    <header class="application-wizard-head">
      <div>
        <p class="kicker">NOVA APLICAÇÃO</p>
        <h1>Nova aplicação</h1>
        <p class="muted">Defina a origem e revise a configuração antes de criar.</p>
      </div>
      <button class="secondary wizard-back" type="button" @click="emit('close')">
        ← Voltar ao projeto
      </button>
    </header>
    <div class="application-wizard-nav">
      <WizardStepper :step="step" />
    </div>
    <section class="application-wizard-body" :class="{ 'application-wizard-body--compose': step === 3 && sourceType === 'compose' }">
      <slot />
    </section>
    <footer class="application-wizard-foot">
      <button v-if="step > 1" class="secondary" type="button" :disabled="busy" @click="emit('back')">Voltar</button>
      <span v-else />
      <div class="wizard-foot-actions">
        <button v-if="step < 4" class="primary" type="button" :disabled="busy || validating || canContinue === false" @click="emit('next')">
          {{ validating ? "Validando…" : step === 3 && sourceType === "compose" ? "Validar e continuar" : "Continuar" }}
        </button>
        <template v-else>
          <button class="secondary" type="button" :disabled="busy" @click="emit('draft')">Salvar como rascunho</button>
          <button class="primary" type="button" :disabled="busy" @click="emit('create')">{{ busy ? "Criando…" : "Criar aplicação" }}</button>
        </template>
      </div>
    </footer>
  </main>
</template>
