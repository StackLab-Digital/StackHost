<script setup lang="ts">
import BaseModal from "../ui/BaseModal.vue";
import WizardStepper from "./WizardStepper.vue";

defineProps<{
  open: boolean;
  editing?: boolean;
  step: number;
  busy?: boolean;
  validating?: boolean;
  sourceType?: string;
}>();
const emit = defineEmits<{
  close: [];
  back: [];
  next: [];
  draft: [];
  create: [];
  save: [];
}>();
</script>

<template>
  <BaseModal
    :open="open"
    :title="editing ? 'Editar aplicação' : 'Nova aplicação'"
    description="Defina a origem e revise a configuração antes de criar."
    @close="emit('close')"
  >
    <div v-if="!editing">
      <WizardStepper :step="step" />
      <slot />
    </div>
    <slot v-else name="edit" />
    <template #footer>
      <button class="secondary" type="button" :disabled="busy" @click="emit('close')">
        Cancelar
      </button>
      <button v-if="!editing && step > 1" class="secondary" type="button" :disabled="busy" @click="emit('back')">
        Voltar
      </button>
      <button v-if="!editing && step < 4" class="primary" type="button" :disabled="busy || validating" @click="emit('next')">
        {{ validating ? "Validando…" : step === 3 && sourceType === "compose" ? "Validar e continuar" : "Continuar" }}
      </button>
      <button v-if="!editing && step === 4" class="secondary" type="button" :disabled="busy" @click="emit('draft')">
        Salvar como rascunho
      </button>
      <button v-if="!editing && step === 4" class="primary" form="new-app-form" type="submit" :disabled="busy">
        {{ busy ? "Criando…" : "Criar aplicação" }}
      </button>
      <button v-if="editing" class="primary" type="button" :disabled="busy" @click="emit('save')">
        {{ busy ? "Salvando…" : "Salvar alterações" }}
      </button>
    </template>
  </BaseModal>
</template>
