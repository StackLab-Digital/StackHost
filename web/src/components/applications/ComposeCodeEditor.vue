<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { yaml } from "@codemirror/lang-yaml";
import {
  defaultKeymap,
  indentWithTab,
  history,
  historyKeymap,
} from "@codemirror/commands";
import { keymap } from "@codemirror/view";
import {
  lineNumbers,
  EditorView,
  highlightActiveLine,
  drawSelection,
} from "@codemirror/view";
import { EditorState } from "@codemirror/state";

const props = withDefaults(
  defineProps<{ modelValue: string; validation?: string }>(),
  { validation: "" },
);
const emit = defineEmits<{
  "update:modelValue": [value: string];
  validate: [];
}>();
const editorHost = ref<HTMLElement | null>(null);
let view: EditorView | null = null;

const example = `services:
  web:
    image: nginx:1.27-alpine
    ports:
      - "8080:80"`;

function createState(value: string) {
  return EditorState.create({
    doc: value,
    extensions: [
      lineNumbers(),
      history(),
      yaml(),
      drawSelection(),
      highlightActiveLine(),
      keymap.of([
        {
          key: "Mod-Enter",
          run: () => {
            emit("validate");
            return true;
          },
        },
        ...defaultKeymap,
        ...historyKeymap,
        indentWithTab,
      ]),
      EditorView.updateListener.of((update) => {
        if (update.docChanged)
          emit("update:modelValue", update.state.doc.toString());
      }),
      EditorView.theme({
        "&": { backgroundColor: "#15171b", color: "#e7eaf0", height: "420px" },
        ".cm-content": {
          fontFamily: "Space Mono, monospace",
          fontSize: "13px",
          padding: "16px 0",
        },
        ".cm-gutters": {
          backgroundColor: "#111317",
          color: "#666d7b",
          border: "0",
        },
        ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "#1d2026" },
        ".cm-scroller": { overflow: "auto" },
      }),
    ],
  });
}
function setValue(value: string) {
  if (!view || view.state.doc.toString() === value) return;
  view.dispatch({
    changes: { from: 0, to: view.state.doc.length, insert: value },
  });
}
function loadExample() {
  setValue(example);
  emit("update:modelValue", example);
}
function clear() {
  setValue("");
  emit("update:modelValue", "");
}
function handleDrop(event: DragEvent) {
  event.preventDefault();
  const file = event.dataTransfer?.files?.[0];
  if (!file || !/\.ya?ml$/i.test(file.name)) return;
  file.text().then((value) => {
    setValue(value);
    emit("update:modelValue", value);
  });
}
onMounted(() => {
  if (editorHost.value) {
    view = new EditorView({
      state: createState(props.modelValue),
      parent: editorHost.value,
    });
  }
});
onBeforeUnmount(() => view?.destroy());
watch(() => props.modelValue, setValue);
</script>

<template>
  <div class="compose-editor-shell">
    <div class="compose-toolbar">
      <span class="compose-file">compose.yml</span>
      <button type="button" class="ghost" @click="loadExample">
        Inserir exemplo
      </button>
      <button type="button" class="ghost" @click="clear">Limpar</button>
    </div>
    <div class="compose-dropzone" @dragover.prevent @drop="handleDrop">
      <div
        ref="editorHost"
        class="compose-editor"
        aria-label="Editor Docker Compose"
      />
      <small
        >Solte um arquivo .yml ou .yaml aqui · Cmd/Ctrl + Enter para
        validar</small
      >
    </div>
    <div class="compose-statusbar">
      <span>YAML</span><span>{{ modelValue.split("\n").length }} linhas</span
      ><span>{{ modelValue.length }} caracteres</span>
    </div>
  </div>
</template>
