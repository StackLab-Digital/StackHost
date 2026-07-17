<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
const props = withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    description?: string;
    busy?: boolean;
    closeOnOverlay?: boolean;
  }>(),
  { closeOnOverlay: true },
);
const emit = defineEmits<{ close: [] }>();
const dialog = ref<HTMLElement | null>(null);
let previous: HTMLElement | null = null;
function close() {
  if (!props.busy) emit("close");
}
function keydown(event: KeyboardEvent) {
  if (event.key === "Escape") close();
}
watch(
  () => props.open,
  async (open) => {
    if (open) {
      previous = document.activeElement as HTMLElement;
      document.body.classList.add("modal-open");
      document.addEventListener("keydown", keydown);
      await nextTick();
      dialog.value?.focus();
    } else {
      document.body.classList.remove("modal-open");
      document.removeEventListener("keydown", keydown);
      previous?.focus();
    }
  },
);
onMounted(() => {
  if (props.open) document.addEventListener("keydown", keydown);
});
onBeforeUnmount(() => {
  document.body.classList.remove("modal-open");
  document.removeEventListener("keydown", keydown);
});
</script>
<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="modal-backdrop"
      @click.self="closeOnOverlay && close()"
    >
      <section
        ref="dialog"
        class="modal"
        role="dialog"
        aria-modal="true"
        :aria-label="title"
        tabindex="-1"
      >
        <header class="modal-head">
          <div>
            <h2>{{ title }}</h2>
            <p v-if="description" class="muted">{{ description }}</p>
          </div>
          <button
            class="icon-button"
            type="button"
            aria-label="Fechar"
            @click="close"
          >
            ×
          </button>
        </header>
        <div class="modal-body"><slot /></div>
        <footer v-if="$slots.footer" class="modal-foot">
          <slot name="footer" />
        </footer>
      </section>
    </div>
  </Teleport>
</template>
