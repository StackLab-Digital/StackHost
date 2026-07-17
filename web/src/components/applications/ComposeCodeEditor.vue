<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { yaml } from "@codemirror/lang-yaml";
import {
  autocompletion,
  closeBrackets,
  closeBracketsKeymap,
} from "@codemirror/autocomplete";
import {
  bracketMatching,
  indentOnInput,
} from "@codemirror/language";
import { searchKeymap, highlightSelectionMatches } from "@codemirror/search";
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
import { MoreHorizontal, FilePlus2, Upload, Check, AlertTriangle, Circle } from "lucide-vue-next";

const props = withDefaults(
  defineProps<{ modelValue: string; validation?: string; fill?: boolean }>(),
  { validation: "" },
);
const emit = defineEmits<{
  "update:modelValue": [value: string];
  validate: [];
  save: [];
  error: [message: string];
}>();
const editorHost = ref<HTMLElement | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);
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
      autocompletion(),
      bracketMatching(),
      closeBrackets(),
      indentOnInput(),
      highlightSelectionMatches(),
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
        {
          key: "Mod-s",
          run: () => {
            emit("save");
            return true;
          },
        },
        ...defaultKeymap,
        ...historyKeymap,
        ...searchKeymap,
        ...closeBracketsKeymap,
        indentWithTab,
      ]),
      EditorView.updateListener.of((update) => {
        if (update.docChanged)
          emit("update:modelValue", update.state.doc.toString());
      }),
      EditorView.theme({
        "&": { backgroundColor: "#15171b", color: "#e7eaf0", height: "100%" },
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
  view.scrollDOM.scrollTop = 0;
  view.focus();
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
  if (!file) return;
  readFile(file);
}
function readFile(file: File) {
  if (!/\.ya?ml$/i.test(file.name)) {
    emit("error", "Selecione um arquivo .yml ou .yaml.");
    return;
  }
  if (file.size > 1024 * 1024) {
    emit("error", "O arquivo Compose deve ter no máximo 1 MB.");
    return;
  }
  file.text().then((value) => {
    setValue(value);
    emit("update:modelValue", value);
  });
}
function chooseFile() {
  fileInput.value?.click();
}
function copyContent() {
  navigator.clipboard?.writeText(props.modelValue);
}
function handleFileInput(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (file) readFile(file);
  input.value = "";
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
  <div class="compose-editor-shell" :class="{ 'compose-editor-shell--fill': fill }">
    <div class="compose-toolbar">
      <span class="compose-file">📄 compose.yml <small>YAML</small></span>
      <span class="compose-toolbar-actions"><button type="button" class="editor-toolbar-button" aria-label="Carregar arquivo Compose" title="Carregar arquivo Compose" @click="chooseFile"><Upload :size="14" /><span>Abrir</span></button><button type="button" class="editor-toolbar-button" title="Inserir exemplo de Docker Compose" @click="loadExample"><FilePlus2 :size="14" /><span>Exemplo</span></button><details class="editor-menu"><summary class="editor-toolbar-button" aria-label="Mais opções"><MoreHorizontal :size="16" /></summary><div class="editor-menu-content"><button type="button" @click="copyContent">Copiar conteúdo</button><button type="button" @click="clear">Limpar editor</button></div></details><span class="compose-validation-status"><Check v-if="validation === 'valid'" :size="14" /> <AlertTriangle v-else-if="validation" :size="14" /> <Circle v-else :size="14" /> {{ validation === 'valid' ? 'Válido' : validation === 'warning' ? 'Avisos' : validation === 'invalid' ? 'Inválido' : 'Não validado' }}</span></span>
    </div>
    <input
      ref="fileInput"
      class="visually-hidden"
      type="file"
      accept=".yml,.yaml,text/yaml"
      @change="handleFileInput"
    />
    <div class="compose-editor-main compose-dropzone" @dragover.prevent @drop="handleDrop">
      <div
        ref="editorHost"
        class="compose-editor"
        aria-label="Editor Docker Compose"
      />
    </div>
    <div class="compose-statusbar">
      <span>Ln 1, Col 1 · {{ modelValue.split("\n").length }} linhas</span><span>YAML · {{ validation === 'valid' ? 'Válido' : validation === 'warning' ? 'Avisos' : validation === 'invalid' ? 'Inválido' : 'Não validado' }}</span>
    </div>
  </div>
</template>
