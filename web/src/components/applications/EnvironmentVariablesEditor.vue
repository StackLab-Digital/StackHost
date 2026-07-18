<script setup lang="ts">
import { ref } from "vue";
import Checkbox from "../ui/Checkbox.vue";
type Variable = {
  key: string;
  value: string;
  secret: boolean;
  has_value?: boolean;
};
type DetectedVariable = { name: string; default_value?: string; required?: boolean; secret?: boolean; services?: string[] };
const props = defineProps<{ modelValue: Variable[]; detected?: DetectedVariable[] }>();
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
function valueFor(item: DetectedVariable) { return props.modelValue.find((variable) => variable.key === item.name); }
</script>
<template>
  <div class="environment-editor">
    <details v-if="detected?.length" class="detected-variables" open>
      <summary>Variáveis detectadas · {{ detected.length }}</summary>
      <div v-for="item in detected" :key="item.name" class="variable-row">
        <strong>{{ item.name }}</strong><span>{{ item.services?.join(", ") || "Compose" }}</span><span>{{ item.secret ? "•••••••• · secret" : valueFor(item)?.value || (item.default_value ? `Padrão: ${item.default_value}` : item.required ? "Valor obrigatório ausente" : "Opcional") }}</span>
      </div>
    </details>
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
      <Checkbox v-model="draft.secret" label="Variável secreta" />
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
