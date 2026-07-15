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

const storageKey = "frida-gui-helper.script-editor-height";
const host = ref<HTMLDivElement | null>(null);
const searchInput = ref<HTMLInputElement | null>(null);
const findVisible = ref(false);
const searchQuery = ref("");
const matchCase = ref(false);
const totalMatches = ref(0);
const currentMatch = ref(0);
const editorHeight = ref(readStoredHeight());

let editor: monaco.editor.IStandaloneCodeEditor | null = null;
let resizeObserver: ResizeObserver | null = null;
let decorations: monaco.editor.IEditorDecorationsCollection | null = null;
let internalUpdate = false;
let matches: monaco.editor.FindMatch[] = [];
let resizing = false;
let resizeStartY = 0;
let resizeStartHeight = 0;

function readStoredHeight() {
  const value = Number(window.localStorage.getItem(storageKey));
  return Number.isFinite(value) ? clampHeight(value) : 420;
}

function clampHeight(value: number) {
  const maxHeight = Math.max(360, Math.min(860, window.innerHeight - 260));
  return Math.min(maxHeight, Math.max(300, Math.round(value)));
}

function layoutEditor() {
  if (!editor || !host.value) {
    return;
  }
  const width = Math.max(0, host.value.clientWidth);
  const height = Math.max(0, host.value.clientHeight);
  if (width > 0 && height > 0) {
    editor.layout({ width, height });
  }
}

function selectedText() {
  const selection = editor?.getSelection();
  const model = editor?.getModel();
  if (!selection || !model || selection.isEmpty()) {
    return "";
  }
  const text = model.getValueInRange(selection);
  return text.includes("\n") ? "" : text;
}

async function showFind() {
  const text = selectedText();
  if (text) {
    searchQuery.value = text;
  }
  findVisible.value = true;
  updateMatches();
  await nextTick();
  searchInput.value?.focus();
  searchInput.value?.select();
}

function hideFind() {
  findVisible.value = false;
  matches = [];
  totalMatches.value = 0;
  currentMatch.value = 0;
  decorations?.clear();
  editor?.focus();
}

function updateMatches() {
  const model = editor?.getModel();
  const query = searchQuery.value;
  if (!editor || !model || !query) {
    matches = [];
    totalMatches.value = 0;
    currentMatch.value = 0;
    decorations?.clear();
    return;
  }

  matches = model.findMatches(query, false, false, matchCase.value, null, true, 999);
  totalMatches.value = matches.length;
  if (matches.length === 0) {
    currentMatch.value = 0;
    decorations?.clear();
    return;
  }
  if (currentMatch.value < 1 || currentMatch.value > matches.length) {
    currentMatch.value = 1;
  }
  renderDecorations();
  revealCurrentMatch();
}

function renderDecorations() {
  if (!decorations) {
    return;
  }
  decorations.set(
    matches.map((match, index) => ({
      range: match.range,
      options: {
        className: index + 1 === currentMatch.value ? "custom-find-match active" : "custom-find-match"
      }
    }))
  );
}

function revealCurrentMatch() {
  if (!editor || currentMatch.value < 1 || currentMatch.value > matches.length) {
    return;
  }
  const match = matches[currentMatch.value - 1];
  editor.setSelection(match.range);
  editor.revealRangeInCenterIfOutsideViewport(match.range);
}

function nextMatch() {
  if (matches.length === 0) {
    return;
  }
  currentMatch.value = currentMatch.value >= matches.length ? 1 : currentMatch.value + 1;
  renderDecorations();
  revealCurrentMatch();
  editor?.focus();
}

function previousMatch() {
  if (matches.length === 0) {
    return;
  }
  currentMatch.value = currentMatch.value <= 1 ? matches.length : currentMatch.value - 1;
  renderDecorations();
  revealCurrentMatch();
  editor?.focus();
}

function startResize(event: PointerEvent) {
  resizing = true;
  resizeStartY = event.clientY;
  resizeStartHeight = editorHeight.value;
  window.addEventListener("pointermove", resizeEditor);
  window.addEventListener("pointerup", stopResize, { once: true });
  event.preventDefault();
}

function resizeEditor(event: PointerEvent) {
  if (!resizing) {
    return;
  }
  editorHeight.value = clampHeight(resizeStartHeight + event.clientY - resizeStartY);
  window.localStorage.setItem(storageKey, String(editorHeight.value));
  requestAnimationFrame(layoutEditor);
}

function stopResize() {
  resizing = false;
  window.removeEventListener("pointermove", resizeEditor);
  requestAnimationFrame(layoutEditor);
}

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
    automaticLayout: false,
    minimap: { enabled: false },
    fontFamily: '"JetBrains Mono", "Cascadia Code", Consolas, monospace',
    fontSize: 13,
    lineHeight: 21,
    tabSize: 4,
    insertSpaces: true,
    scrollBeyondLastLine: false,
    wordWrap: "off",
    renderWhitespace: "selection",
    fixedOverflowWidgets: false,
    guides: { indentation: true },
    suggest: { snippetsPreventQuickSuggestions: false }
  });

  decorations = editor.createDecorationsCollection();
  editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyF, () => {
    void showFind();
  });
  editor.addCommand(monaco.KeyCode.Escape, () => {
    if (findVisible.value) {
      hideFind();
    }
  });
  editor.onDidChangeModelContent(() => {
    if (!editor || internalUpdate) {
      return;
    }
    emit("update:modelValue", editor.getValue());
    if (findVisible.value) {
      updateMatches();
    }
  });

  resizeObserver = new ResizeObserver(() => layoutEditor());
  resizeObserver.observe(host.value);
  layoutEditor();

  emit("ready", {
    focus: () => editor?.focus(),
    format: async () => {
      await editor?.getAction("editor.action.formatDocument")?.run();
    },
    find: () => {
      void showFind();
    },
    replace: (source: string) => {
      if (!editor) {
        return;
      }
      internalUpdate = true;
      editor.setValue(source);
      internalUpdate = false;
      emit("update:modelValue", source);
      if (findVisible.value) {
        updateMatches();
      }
    }
  });
});

watch(searchQuery, () => updateMatches());
watch(matchCase, () => updateMatches());
watch(editorHeight, () => requestAnimationFrame(layoutEditor));

watch(
  () => props.modelValue,
  (value) => {
    if (!editor || value === editor.getValue()) {
      return;
    }
    internalUpdate = true;
    editor.setValue(value);
    internalUpdate = false;
    if (findVisible.value) {
      updateMatches();
    }
  }
);

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  decorations?.clear();
  editor?.dispose();
  window.removeEventListener("pointermove", resizeEditor);
});
</script>

<template>
  <div class="script-editor-frame" :style="{ height: `${editorHeight}px` }">
    <div v-if="findVisible" class="custom-find" @keydown.stop>
      <input
        ref="searchInput"
        v-model="searchQuery"
        type="search"
        placeholder="查找脚本"
        @keydown.enter.prevent="nextMatch"
        @keydown.shift.enter.prevent="previousMatch"
        @keydown.escape.prevent="hideFind"
      />
      <span class="find-count">{{ totalMatches === 0 ? '0/0' : `${currentMatch}/${totalMatches}` }}</span>
      <label class="find-case" title="区分大小写">
        <input v-model="matchCase" type="checkbox" />
        <span>Aa</span>
      </label>
      <button type="button" title="上一个" @click="previousMatch">↑</button>
      <button type="button" title="下一个" @click="nextMatch">↓</button>
      <button type="button" title="关闭查找" @click="hideFind">×</button>
    </div>
    <div ref="host" class="script-editor"></div>
    <div class="resize-handle" title="拖拽调整编辑器高度" @pointerdown="startResize">
      <span></span>
    </div>
  </div>
</template>

<style scoped>
.script-editor-frame {
  position: relative;
  display: grid;
  min-width: 0;
  min-height: 300px;
  grid-template-rows: minmax(0, 1fr) 8px;
  overflow: hidden;
  background: #1e1e1e;
}

.script-editor {
  position: relative;
  width: 100%;
  min-width: 0;
  min-height: 0;
  height: 100%;
  overflow: hidden;
  contain: layout size;
  background: #1e1e1e;
}

.custom-find {
  position: absolute;
  top: 10px;
  right: 12px;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 6px;
  max-width: calc(100% - 24px);
  min-height: 36px;
  padding: 5px;
  color: #d7dde3;
  background: #2b2f33;
  border: 1px solid #49515a;
  border-radius: 7px;
  box-shadow: 0 8px 18px rgba(0, 0, 0, 0.35);
}

.custom-find input[type="search"] {
  width: min(260px, 34vw);
  height: 28px;
  padding: 0 8px;
  color: #eef3f6;
  background: #1f2327;
  border: 1px solid #555e68;
  border-radius: 5px;
  outline: none;
}

.custom-find input[type="search"]:focus {
  border-color: #78c7b2;
}

.find-count {
  min-width: 46px;
  color: #adb8c2;
  font-size: 12px;
  text-align: center;
}

.find-case {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 28px;
  color: #d7dde3;
  font-size: 12px;
}

.find-case input {
  width: 14px;
  height: 14px;
}

.custom-find button {
  width: 28px;
  height: 28px;
  color: #e8edf1;
  background: #373d43;
  border: 1px solid #555e68;
  border-radius: 5px;
}

.custom-find button:hover {
  background: #46505a;
}

.resize-handle {
  display: grid;
  height: 8px;
  place-items: center;
  cursor: ns-resize;
  background: #15191d;
  border-top: 1px solid #2f373d;
}

.resize-handle span {
  width: 44px;
  height: 3px;
  background: #68737d;
  border-radius: 999px;
}

.resize-handle:hover span {
  background: #78c7b2;
}

.script-editor :deep(.monaco-editor),
.script-editor :deep(.overflow-guard) {
  width: 100% !important;
  height: 100% !important;
}

.script-editor :deep(.custom-find-match) {
  background: rgba(255, 213, 79, 0.28);
  outline: 1px solid rgba(255, 213, 79, 0.42);
}

.script-editor :deep(.custom-find-match.active) {
  background: rgba(22, 124, 100, 0.55);
  outline: 1px solid rgba(120, 199, 178, 0.95);
}
</style>