<script setup lang="ts">
import { ref } from "vue";
type Variable = {
  key: string;
  value: string;
  secret: boolean;
  has_value?: boolean;
};
const props = defineProps<{ modelValue: Variable[] }>();
const emit = defineEmits<{ "update:modelValue": [value: Variable[]] }>();
const draft = ref<Variable>({ key: "", value: "", secret: false });
function add() {
  if (!draft.value.key.trim()) return;
  emit("update:modelValue", [
    ...props.modelValue,
    { ...draft.value, key: draft.value.key.trim() },
  ]);
  draft.value = { key: "", value: "", secret: false };
}
function remove(index: number) {
  emit(
    "update:modelValue",
    props.modelValue.filter((_, itemIndex) => itemIndex !== index),
  );
}
</script>
<template>
  <div class="environment-editor">
    <h3>Variáveis de ambiente</h3>
    <div class="variable-row new-variable">
      <input
        v-model="draft.key"
        placeholder="CHAVE"
        aria-label="Nova variável"
      />
      <input
        v-model="draft.value"
        placeholder="Valor"
        :type="draft.secret ? 'password' : 'text'"
      />
      <label class="check"
        ><input v-model="draft.secret" type="checkbox" /> Secret</label
      >
      <button class="secondary" type="button" @click="add">Adicionar</button>
    </div>
    <p v-if="!modelValue.length" class="muted">Nenhuma variável configurada.</p>
    <div
      v-for="(item, index) in modelValue"
      :key="`${item.key}-${index}`"
      class="variable-row"
    >
      <strong>{{ item.key }}</strong
      ><span>{{ item.secret ? "•••••••• · valor protegido" : item.value }}</span
      ><button class="ghost" type="button" @click="remove(index)">
        Remover
      </button>
    </div>
  </div>
</template>
