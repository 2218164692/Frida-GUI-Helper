<script setup lang="ts">
import { Clipboard, RefreshCw, RotateCcw, Square, Terminal } from "@lucide/vue";
import { onMounted, onUnmounted, ref, watch } from "vue";
import type { SessionInfo, SessionLogEntry } from "../lib/api";

const props = defineProps<{
  sessions: SessionInfo[];
  selectedId: string;
  logs: SessionLogEntry[];
  busy: string;
}>();

const emit = defineEmits<{
  refresh: [];
  select: [session: SessionInfo];
  stop: [session: SessionInfo];
  restart: [session: SessionInfo];
  copyError: [session: SessionInfo];
}>();

const now = ref(Date.now());
let clock: number | undefined;

function startClock() {
  if (clock !== undefined) return;
  now.value = Date.now();
  clock = window.setInterval(() => { now.value = Date.now(); }, 1000);
}

function stopClock() {
  if (clock === undefined) return;
  window.clearInterval(clock);
  clock = undefined;
}

function syncClock(running: boolean) {
  if (running) startClock();
  else stopClock();
}

onMounted(() => syncClock(props.sessions.some((session) => session.running)));
watch(() => props.sessions.some((session) => session.running), syncClock);
onUnmounted(stopClock);

function duration(session: SessionInfo) {
  const value = session.running ? Math.max(0, now.value - new Date(session.startedAt).getTime()) : session.durationMs;
  if (!Number.isFinite(value)) return "-";
  if (value < 1000) return `${Math.round(value)} ms`;
  const seconds = Math.round(value / 1000);
  return seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`;
}

function timestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString("zh-CN", { hour12: false });
}

function selectedSession() {
  return props.sessions.find((session) => session.id === props.selectedId);
}
</script>

<template>
  <div class="session-panel">
    <div class="session-header">
      <div>
        <h2>会话与独立日志</h2>
        <p>{{ sessions.filter((item) => item.running).length }} 个运行中 · 保留最近完成记录</p>
      </div>
      <button type="button" title="刷新会话" @click="emit('refresh')"><RefreshCw :size="16" /></button>
    </div>

    <div class="session-layout">
      <div class="session-list">
        <div v-if="sessions.length === 0" class="empty">暂无会话</div>
        <button
          v-for="session in sessions"
          :key="session.id"
          class="session-row"
          :class="{ active: session.id === selectedId, running: session.running, stopping: session.stopping, failed: !session.running && Boolean(session.error) }"
          type="button"
          @click="emit('select', session)"
        >
          <span class="status-dot"></span>
          <span class="session-main">
            <strong>{{ session.scriptName }}</strong>
            <small>{{ session.mode }} · {{ session.target }} · {{ duration(session) }}</small>
          </span>
          <span class="session-state">{{ session.stopping ? "停止中" : (session.running ? "运行中" : (session.exitReason || "已结束")) }}</span>
        </button>
      </div>

      <div class="session-detail">
        <template v-if="selectedSession()">
          <div class="detail-actions">
            <div>
              <strong>{{ selectedSession()!.scriptName }}</strong>
              <span>{{ selectedSession()!.id }} · {{ logs.length }} 条输出</span>
            </div>
            <button
              v-if="selectedSession()!.running"
              class="danger"
              type="button"
              :disabled="selectedSession()!.stopping || busy === `stop-${selectedSession()!.id}`"
              @click="emit('stop', selectedSession()!)"
            ><Square :size="15" /> {{ selectedSession()!.stopping ? "停止中…" : "停止" }}</button>
            <button
              v-else
              class="strong"
              type="button"
              :disabled="busy === `restart-${selectedSession()!.id}`"
              @click="emit('restart', selectedSession()!)"
            ><RotateCcw :size="15" /> 重新注入</button>
            <button type="button" @click="emit('copyError', selectedSession()!)"><Clipboard :size="15" /> 复制错误</button>
          </div>
          <div v-if="selectedSession()!.error || selectedSession()!.exitReason" class="exit-summary">
            <strong>{{ selectedSession()!.exitReason || "会话已退出" }}</strong>
            <span v-if="selectedSession()!.exitCode != null">退出码 {{ selectedSession()!.exitCode }}</span>
            <small v-if="selectedSession()!.error">{{ selectedSession()!.error }}</small>
          </div>
          <div class="session-console">
            <div v-if="logs.length === 0" class="empty"><Terminal :size="17" /> 暂无 stdout/stderr</div>
            <div v-for="entry in logs" :key="`${entry.sessionId}:${entry.sequence}`" class="console-line" :class="entry.stream">
              <time>{{ timestamp(entry.timestamp) }}</time>
              <b>{{ entry.stream }}</b>
              <pre>{{ entry.message }}</pre>
            </div>
          </div>
        </template>
        <div v-else class="empty detail-empty"><Terminal :size="20" /> 选择会话查看独立输出</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.session-panel { background: #fff; border: 1px solid #d7e0e4; border-radius: 9px; min-width: 0; min-height: 320px; height: auto; flex: 1 1 0; display: flex; flex-direction: column; overflow: hidden; }
.session-header { min-height: 59px; padding: 0 15px; display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid #e4eaed; }
h2 { margin: 0; font-size: 16px; color: #1c3038; }
p { margin: 3px 0 0; color: #74858c; font-size: 11px; }
button { border: 1px solid #cad5da; border-radius: 6px; background: #fff; color: #40545d; min-height: 32px; padding: 0 9px; display: inline-flex; align-items: center; justify-content: center; gap: 6px; cursor: pointer; }
button:disabled { opacity: .5; cursor: not-allowed; }
.session-layout { display: grid; grid-template-columns: minmax(220px, 34%) minmax(0, 1fr); min-width: 0; min-height: 0; height: auto; flex: 1 1 0; overflow: hidden; }
.session-list { min-width: 0; min-height: 0; border-right: 1px solid #e3e9ec; overflow: auto; padding: 7px; }
.session-row { width: 100%; min-height: 55px; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; text-align: left; gap: 8px; border: 0; border-radius: 6px; padding: 8px; }
.session-row:hover, .session-row.active { background: #edf7f4; }
.status-dot { width: 8px; height: 8px; margin-top: 5px; border-radius: 50%; background: #9aa8ae; }
.session-row.running .status-dot { background: #188b70; box-shadow: 0 0 0 3px rgba(24,139,112,.12); }
.session-row.stopping .status-dot { background: #c58b35; box-shadow: 0 0 0 3px rgba(197,139,53,.14); }
.session-row.failed .status-dot { background: #c24f43; }
.session-main { min-width: 0; display: grid; gap: 3px; }
.session-main strong, .session-main small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.session-main strong { color: #263a43; }
.session-main small, .session-state { color: #74858c; font-size: 10px; }
.session-detail { min-width: 0; min-height: 0; display: grid; grid-template-rows: auto auto minmax(0, 1fr); overflow: hidden; }
.detail-actions { min-height: 48px; padding: 7px 10px; display: flex; gap: 7px; align-items: center; border-bottom: 1px solid #e4eaed; }
.detail-actions > div { min-width: 0; flex: 1; display: grid; gap: 2px; }
.detail-actions > div span { color: #718188; font-size: 10px; }
.detail-actions button.strong { color: #fff; background: #168a73; border-color: #168a73; }
.detail-actions button.danger { color: #b13e3e; }
.exit-summary { display: flex; gap: 8px; align-items: center; padding: 7px 10px; color: #6e7d83; background: #fff8f4; font-size: 11px; }
.exit-summary small { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #b14c3e; }
.session-console { min-width: 0; min-height: 0; height: auto; overflow: auto; padding: 9px 10px; background: #10171b; color: #d9e2e5; font: 11px/1.55 Consolas, "Cascadia Mono", monospace; }
.console-line { display: grid; grid-template-columns: 66px 48px minmax(0, 1fr); gap: 7px; align-items: start; }
.console-line time { color: #64767f; }
.console-line b { color: #54b79f; font-weight: 500; }
.console-line.stderr b, .console-line.stderr pre { color: #ff9885; }
.console-line pre { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: inherit; }
.empty { min-height: 90px; color: #819097; display: flex; align-items: center; justify-content: center; gap: 7px; }
.detail-empty { min-height: 270px; }
@media (max-width: 860px) {
  .session-panel { min-height: 520px; }
  .session-layout { grid-template-columns: 1fr; grid-template-rows: minmax(145px, 30%) minmax(300px, 1fr); overflow: auto; }
  .session-list { border-right: 0; border-bottom: 1px solid #e3e9ec; }
}
</style>
