<script setup lang="ts">
import { computed } from "vue";
import {
  Bell,
  CheckCircle2,
  Download,
  ExternalLink,
  Folder,
  Package,
  RefreshCw,
  ShieldCheck
} from "@lucide/vue";

interface UpdateInfo {
  currentVersion: string;
  latestVersion?: string;
  updateAvailable?: boolean;
  releaseUrl?: string;
  checkedAt?: string;
  error?: string;
}

interface UpdatePreferences {
  autoCheckAppUpdates: boolean;
  autoCheckFridaUpdates: boolean;
  autoUpdateFrida: boolean;
  notifyWhenUpdateAvailable: boolean;
  autoPreflightBeforeInject: boolean;
}

interface Props {
  appUpdate: UpdateInfo;
  fridaUpdate: UpdateInfo;
  installDirectory: string;
  fridaDirectory: string;
  preferences: UpdatePreferences;
  checkingApp?: boolean;
  checkingFrida?: boolean;
  updatingFrida?: boolean;
  savingPreferences?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  checkingApp: false,
  checkingFrida: false,
  updatingFrida: false,
  savingPreferences: false
});

const emit = defineEmits<{
  checkAppUpdate: [];
  openAppDownload: [url: string];
  checkFridaUpdate: [];
  updateFrida: [];
  updatePreferences: [preferences: UpdatePreferences];
}>();

const appStatusLabel = computed(() => updateStatusLabel(props.appUpdate));
const fridaStatusLabel = computed(() => updateStatusLabel(props.fridaUpdate));

function updateStatusLabel(update: UpdateInfo): string {
  if (update.error) {
    return "检查失败";
  }
  if (update.updateAvailable) {
    return "发现新版本";
  }
  if (update.checkedAt) {
    return "已是最新版本";
  }
  return "尚未检查";
}

function statusClass(update: UpdateInfo): string {
  if (update.error) {
    return "error";
  }
  if (update.updateAvailable) {
    return "available";
  }
  if (update.checkedAt) {
    return "current";
  }
  return "idle";
}

function versionLabel(update: UpdateInfo): string {
  if (!update.latestVersion) {
    return "等待检查";
  }
  return update.updateAvailable ? `最新 ${update.latestVersion}` : `远端 ${update.latestVersion}`;
}

function formatCheckedAt(value?: string): string {
  if (!value) {
    return "尚未执行在线检查";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return `上次检查：${value}`;
  }
  return `上次检查：${date.toLocaleString("zh-CN", { hour12: false })}`;
}

function setPreference(key: keyof UpdatePreferences, value: boolean) {
  emit("updatePreferences", {
    ...props.preferences,
    [key]: value
  });
}
</script>

<template>
  <section class="settings-workspace" aria-label="设置">
    <div class="settings-grid">
      <article class="settings-panel update-panel">
        <header class="panel-header">
          <div class="panel-heading">
            <span class="heading-icon"><Package :size="18" /></span>
            <div>
              <h2>软件更新</h2>
              <p>检查 Frida GUI Helper 的新安装包</p>
            </div>
          </div>
          <span class="status-pill" :class="statusClass(appUpdate)">{{ appStatusLabel }}</span>
        </header>

        <div class="update-body">
          <div class="version-row">
            <div>
              <span>当前版本</span>
              <strong>{{ appUpdate.currentVersion || "-" }}</strong>
            </div>
            <div class="version-divider"></div>
            <div>
              <span>可用版本</span>
              <strong>{{ versionLabel(appUpdate) }}</strong>
            </div>
          </div>

          <div v-if="appUpdate.error" class="inline-message error" role="alert">
            {{ appUpdate.error }}
          </div>
          <div v-else-if="appUpdate.updateAvailable" class="inline-message available">
            新版安装包已发布，下载后可选择原目录覆盖安装。
          </div>
          <div v-else class="inline-message neutral">
            {{ formatCheckedAt(appUpdate.checkedAt) }}
          </div>

          <div class="card-actions">
            <button
              class="secondary-button"
              type="button"
              :disabled="checkingApp"
              @click="emit('checkAppUpdate')"
            >
              <RefreshCw :size="16" :class="{ spin: checkingApp }" />
              <span>{{ checkingApp ? "正在检查…" : "检查更新" }}</span>
            </button>
            <button
              v-if="appUpdate.updateAvailable"
              class="primary-button"
              type="button"
              :disabled="!appUpdate.releaseUrl"
              @click="emit('openAppDownload', appUpdate.releaseUrl || '')"
            >
              <ExternalLink :size="16" />
              <span>前往下载</span>
            </button>
          </div>
        </div>
      </article>

      <article class="settings-panel update-panel">
        <header class="panel-header">
          <div class="panel-heading">
            <span class="heading-icon"><ShieldCheck :size="18" /></span>
            <div>
              <h2>Frida 组件</h2>
              <p>维护软件内置的 CLI 与 Android frida-server</p>
            </div>
          </div>
          <span class="status-pill" :class="statusClass(fridaUpdate)">{{ fridaStatusLabel }}</span>
        </header>

        <div class="update-body">
          <div class="version-row">
            <div>
              <span>内置版本</span>
              <strong>{{ fridaUpdate.currentVersion || "-" }}</strong>
            </div>
            <div class="version-divider"></div>
            <div>
              <span>可用版本</span>
              <strong>{{ versionLabel(fridaUpdate) }}</strong>
            </div>
          </div>

          <div v-if="fridaUpdate.error" class="inline-message error" role="alert">
            {{ fridaUpdate.error }}
          </div>
          <div v-else-if="fridaUpdate.updateAvailable" class="inline-message available">
            可更新软件目录内的 Frida 组件；不会读取或修改系统 PATH 中的 Frida。
          </div>
          <div v-else class="inline-message neutral">
            {{ formatCheckedAt(fridaUpdate.checkedAt) }}
          </div>

          <div class="card-actions">
            <button
              class="secondary-button"
              type="button"
              :disabled="checkingFrida || updatingFrida"
              @click="emit('checkFridaUpdate')"
            >
              <RefreshCw :size="16" :class="{ spin: checkingFrida }" />
              <span>{{ checkingFrida ? "正在检查…" : "检查更新" }}</span>
            </button>
            <button
              v-if="fridaUpdate.updateAvailable"
              class="primary-button"
              type="button"
              :disabled="updatingFrida"
              @click="emit('updateFrida')"
            >
              <Download :size="16" :class="{ pulse: updatingFrida }" />
              <span>{{ updatingFrida ? "正在更新…" : "更新 Frida" }}</span>
            </button>
          </div>
        </div>
      </article>

      <article class="settings-panel paths-panel">
        <header class="panel-header">
          <div class="panel-heading">
            <span class="heading-icon"><Folder :size="18" /></span>
            <div>
              <h2>安装与组件目录</h2>
              <p>运行时只使用安装目录内的 Frida 工具链</p>
            </div>
          </div>
        </header>

        <div class="path-list">
          <div class="path-row">
            <span>软件安装目录</span>
            <code :title="installDirectory">{{ installDirectory || "-" }}</code>
          </div>
          <div class="path-row">
            <span>Frida 组件目录</span>
            <code :title="fridaDirectory">{{ fridaDirectory || "-" }}</code>
          </div>
        </div>
      </article>

      <article class="settings-panel preferences-panel">
        <header class="panel-header">
          <div class="panel-heading">
            <span class="heading-icon"><Bell :size="18" /></span>
            <div>
              <h2>更新偏好</h2>
              <p>选择软件启动后如何检查和处理更新</p>
            </div>
          </div>
          <span v-if="savingPreferences" class="saving-label">正在保存…</span>
        </header>

        <div class="preference-list">
          <div class="preference-row">
            <div>
              <strong>自动检查软件更新</strong>
              <span>启动后在后台检查新的安装包</span>
            </div>
            <button
              class="switch"
              :class="{ enabled: preferences.autoCheckAppUpdates }"
              type="button"
              role="switch"
              :aria-checked="preferences.autoCheckAppUpdates"
              :disabled="savingPreferences"
              @click="setPreference('autoCheckAppUpdates', !preferences.autoCheckAppUpdates)"
            ><span></span></button>
          </div>

          <div class="preference-row">
            <div>
              <strong>自动检查 Frida 更新</strong>
              <span>检查官方稳定版是否高于软件内置版本</span>
            </div>
            <button
              class="switch"
              :class="{ enabled: preferences.autoCheckFridaUpdates }"
              type="button"
              role="switch"
              :aria-checked="preferences.autoCheckFridaUpdates"
              :disabled="savingPreferences"
              @click="setPreference('autoCheckFridaUpdates', !preferences.autoCheckFridaUpdates)"
            ><span></span></button>
          </div>

          <div class="preference-row">
            <div>
              <strong>自动更新 Frida 组件</strong>
              <span>发现稳定版后下载并替换安装目录内的组件</span>
            </div>
            <button
              class="switch"
              :class="{ enabled: preferences.autoUpdateFrida }"
              type="button"
              role="switch"
              :aria-checked="preferences.autoUpdateFrida"
              :disabled="savingPreferences"
              @click="setPreference('autoUpdateFrida', !preferences.autoUpdateFrida)"
            ><span></span></button>
          </div>

          <div class="preference-row">
            <div>
              <strong>发现更新时通知</strong>
              <span>在界面中显示可用版本提示</span>
            </div>
            <button
              class="switch"
              :class="{ enabled: preferences.notifyWhenUpdateAvailable }"
              type="button"
              role="switch"
              :aria-checked="preferences.notifyWhenUpdateAvailable"
              :disabled="savingPreferences"
              @click="setPreference('notifyWhenUpdateAvailable', !preferences.notifyWhenUpdateAvailable)"
            ><span></span></button>
          </div>

          <div class="preference-row">
            <div>
              <strong>注入前自动检查环境</strong>
              <span>开启后每次注入都会检查设备、Root、ABI、server 和目标进程；关闭后可手动检查</span>
            </div>
            <button
              class="switch"
              :class="{ enabled: preferences.autoPreflightBeforeInject }"
              type="button"
              role="switch"
              :aria-checked="preferences.autoPreflightBeforeInject"
              :disabled="savingPreferences"
              @click="setPreference('autoPreflightBeforeInject', !preferences.autoPreflightBeforeInject)"
            ><span></span></button>
          </div>
        </div>
      </article>
    </div>

    <div class="settings-note">
      <CheckCircle2 :size="17" />
      <span>系统中已有的 Frida 不会参与版本检测、脚本运行或手机端部署。</span>
    </div>
  </section>
</template>

<style scoped>
.settings-workspace {
  min-height: 0;
  padding: 18px 22px 22px;
  overflow: auto;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.settings-panel {
  min-width: 0;
  overflow: hidden;
  background: #ffffff;
  border: 1px solid #dde3ea;
  border-radius: 8px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 68px;
  padding: 14px 16px;
  border-bottom: 1px solid #e7ebf0;
}

.panel-heading {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 10px;
}

.panel-heading > div {
  min-width: 0;
}

.heading-icon {
  display: grid;
  width: 34px;
  height: 34px;
  flex: 0 0 auto;
  color: #167c64;
  background: #e7f5f0;
  border-radius: 8px;
  place-items: center;
}

.panel-header h2 {
  margin: 0;
  color: #20252d;
  font-size: 16px;
  line-height: 1.25;
}

.panel-header p {
  margin: 4px 0 0;
  overflow: hidden;
  color: #6d7985;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status-pill {
  min-width: 72px;
  flex: 0 0 auto;
  padding: 5px 9px;
  color: #65717c;
  font-size: 11px;
  font-weight: 700;
  text-align: center;
  background: #eef2f5;
  border-radius: 99px;
}

.status-pill.available {
  color: #7d4b08;
  background: #fff1cf;
}

.status-pill.current {
  color: #0f684f;
  background: #e3f5ee;
}

.status-pill.error {
  color: #99382d;
  background: #fff0ed;
}

.update-body {
  display: grid;
  gap: 14px;
  padding: 16px;
}

.version-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 1px minmax(0, 1fr);
  gap: 16px;
  align-items: stretch;
  padding: 2px 0;
}

.version-row > div:not(.version-divider) {
  display: grid;
  gap: 5px;
}

.version-row span {
  color: #6d7985;
  font-size: 11px;
}

.version-row strong {
  overflow: hidden;
  color: #20252d;
  font-size: 17px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.version-divider {
  width: 1px;
  background: #e3e8ed;
}

.inline-message {
  min-height: 36px;
  padding: 9px 11px;
  color: #596772;
  font-size: 12px;
  line-height: 1.45;
  background: #f5f7f9;
  border: 1px solid #e1e6eb;
  border-radius: 7px;
}

.inline-message.available {
  color: #72501c;
  background: #fff9e9;
  border-color: #ecd9a6;
}

.inline-message.error {
  color: #92392f;
  background: #fff3f0;
  border-color: #f0c8c1;
}

.card-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.primary-button,
.secondary-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 36px;
  padding: 0 13px;
  color: #27313b;
  font: inherit;
  background: #ffffff;
  border: 1px solid #cad2dc;
  border-radius: 7px;
}

.primary-button {
  color: #ffffff;
  background: #167c64;
  border-color: #167c64;
}

.primary-button:hover:not(:disabled) {
  background: #126b56;
}

button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.paths-panel,
.preferences-panel {
  grid-column: 1 / -1;
}

.path-list {
  display: grid;
  gap: 0;
}

.path-row {
  display: grid;
  grid-template-columns: 160px minmax(0, 1fr);
  align-items: center;
  min-height: 56px;
  gap: 14px;
  padding: 10px 16px;
  border-bottom: 1px solid #edf0f3;
}

.path-row:last-child {
  border-bottom: 0;
}

.path-row > span {
  color: #5f6c77;
  font-size: 12px;
  font-weight: 600;
}

.path-row code {
  display: block;
  min-width: 0;
  padding: 8px 10px;
  overflow: hidden;
  color: #35434e;
  font-family: "Cascadia Code", Consolas, monospace;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: #f5f7f9;
  border: 1px solid #e0e5ea;
  border-radius: 6px;
}

.saving-label {
  flex: 0 0 auto;
  color: #6d7985;
  font-size: 11px;
}

.preference-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.preference-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-width: 0;
  min-height: 76px;
  gap: 16px;
  padding: 14px 16px;
  border-right: 1px solid #edf0f3;
  border-bottom: 1px solid #edf0f3;
}

.preference-row:nth-child(2n) {
  border-right: 0;
}

.preference-row:nth-last-child(-n + 2) {
  border-bottom: 0;
}

.preference-row > div {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.preference-row strong {
  color: #26313a;
  font-size: 13px;
}

.preference-row span {
  color: #6d7985;
  font-size: 11px;
  line-height: 1.4;
}

.switch {
  position: relative;
  width: 42px;
  height: 24px;
  flex: 0 0 auto;
  padding: 0;
  background: #b5bec6;
  border: 0;
  border-radius: 99px;
  transition: background 0.18s ease;
}

.switch span {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 18px;
  height: 18px;
  background: #ffffff;
  border-radius: 50%;
  box-shadow: 0 1px 3px rgba(29, 37, 43, 0.28);
  transition: transform 0.18s ease;
}

.switch.enabled {
  background: #167c64;
}

.switch.enabled span {
  transform: translateX(18px);
}

.settings-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
  padding: 10px 13px;
  color: #42685d;
  font-size: 12px;
  background: #edf8f4;
  border: 1px solid #c9e8dc;
  border-radius: 8px;
}

.settings-note svg {
  flex: 0 0 auto;
  color: #167c64;
}

.spin {
  animation: spin 0.8s linear infinite;
}

.pulse {
  animation: pulse 0.9s ease-in-out infinite alternate;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes pulse {
  to {
    opacity: 0.45;
  }
}

@media (max-width: 900px) {
  .settings-grid,
  .preference-list {
    grid-template-columns: minmax(0, 1fr);
  }

  .paths-panel,
  .preferences-panel {
    grid-column: auto;
  }

  .preference-row,
  .preference-row:nth-child(2n),
  .preference-row:nth-last-child(-n + 2) {
    border-right: 0;
    border-bottom: 1px solid #edf0f3;
  }

  .preference-row:last-child {
    border-bottom: 0;
  }

  .path-row {
    grid-template-columns: minmax(0, 1fr);
    gap: 7px;
  }
}
</style>
