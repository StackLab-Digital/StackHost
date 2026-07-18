<script setup lang="ts">
defineProps<{
  modelValue: boolean;
  label: string;
  disabled?: boolean;
}>();

const emit = defineEmits<{ "update:modelValue": [value: boolean] }>();
</script>

<template>
  <label class="checkbox-control" :class="{ disabled }">
    <input
      type="checkbox"
      :checked="modelValue"
      :disabled="disabled"
      @change="emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
    />
    <span>{{ label }}</span>
  </label>
</template>

<style scoped>
.checkbox-control {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: #b9bec9;
  cursor: pointer;
  font-size: 14px;
  line-height: 1.2;
}
.checkbox-control.disabled { cursor: not-allowed; opacity: 0.55; }
.checkbox-control input {
  appearance: none;
  width: 48px;
  height: 48px;
  flex: 0 0 48px;
  margin: 0;
  padding: 0;
  border: 1px solid #454a55;
  border-radius: 7px;
  background: #191c21;
  cursor: inherit;
}
.checkbox-control input:checked {
  border-color: #e8f55b;
  background-color: #e8f55b;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Cpath d='m5 12 4 4L19 6' fill='none' stroke='%23151616' stroke-linecap='round' stroke-linejoin='round' stroke-width='3'/%3E%3C/svg%3E");
  background-position: center;
  background-repeat: no-repeat;
  background-size: 23px;
}
.checkbox-control input:focus-visible {
  outline: 2px solid #e8f55b;
  outline-offset: -2px;
}
</style>
