<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import "monaco-editor/esm/vs/language/typescript/monaco.contribution";
import editorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";
import tsWorker from "monaco-editor/esm/vs/language/typescript/ts.worker?worker";

const props = defineProps<{
  modelValue: string;
  language?: string;
}>();

interface ScriptEditorApi {
  focus: () => void;
  format: () => Promise<void>;
  find: () => void;
  replace: (source: string) => void;
}

const emit = defineEmits<{
  "update:modelValue": [value: string];
  ready: [api: ScriptEditorApi];
}>();

const host = ref<HTMLDivElement | null>(null);
let editor: monaco.editor.IStandaloneCodeEditor | null = null;
let resizeObserver: ResizeObserver | null = null;
let internalUpdate = false;

(self as unknown as { MonacoEnvironment: monaco.Environment }).MonacoEnvironment = {
  getWorker(_workerId: string, label: string) {
    if (label === "typescript" || label === "javascript") {
      return new tsWorker();
    }
    return new editorWorker();
  }
};

onMounted(async () => {
  await nextTick();
  if (!host.value) {
    return;
  }

  editor = monaco.editor.create(host.value, {
    value: props.modelValue,
    language: props.language || "javascript",
    theme: "vs-dark",
    automaticLayout: true,
    minimap: { enabled: false },
    fontFamily: '"JetBrains Mono", "Cascadia Code", Consolas, monospace',
    fontSize: 13,
    lineHeight: 21,
    tabSize: 4,
    insertSpaces: true,
    scrollBeyondLastLine: false,
    wordWrap: "off",
    renderWhitespace: "selection",
    guides: { indentation: true },
    suggest: { snippetsPreventQuickSuggestions: false }
  });

  editor.onDidChangeModelContent(() => {
    if (!editor || internalUpdate) {
      return;
    }
    emit("update:modelValue", editor.getValue());
  });

  resizeObserver = new ResizeObserver(() => editor?.layout());
  resizeObserver.observe(host.value);

  emit("ready", {
    focus: () => editor?.focus(),
    format: async () => {
      await editor?.getAction("editor.action.formatDocument")?.run();
    },
    find: () => editor?.getAction("actions.find")?.run(),
    replace: (source: string) => {
      if (!editor) {
        return;
      }
      internalUpdate = true;
      editor.setValue(source);
      internalUpdate = false;
      emit("update:modelValue", source);
    }
  });
});

watch(
  () => props.modelValue,
  (value) => {
    if (!editor || value === editor.getValue()) {
      return;
    }
    internalUpdate = true;
    editor.setValue(value);
    internalUpdate = false;
  }
);

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  editor?.dispose();
});
</script>

<template>
  <div ref="host" class="script-editor"></div>
</template>

<style scoped>
.script-editor {
  width: 100%;
  min-width: 0;
  min-height: 360px;
  height: 100%;
  background: #1e1e1e;
}
</style>
