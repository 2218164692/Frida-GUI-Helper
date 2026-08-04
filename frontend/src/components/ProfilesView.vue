<script setup lang="ts">
import { Clock3, Play, Save, Trash2, Upload } from "@lucide/vue";
import type { RunProfile } from "../lib/api";

defineProps<{
  profiles: RunProfile[];
  lastProfile: RunProfile | null;
  draftName: string;
  busy: string;
  canSave: boolean;
}>();

const emit = defineEmits<{
  "update:draftName": [value: string];
  save: [];
  load: [profile: RunProfile];
  run: [profile: RunProfile];
  delete: [profile: RunProfile];
}>();

function formatDate(value: string) {
  if (!value) return "尚未运行";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}
</script>

<template>
  <section class="profiles-workspace">
    <div class="quick-grid">
      <article class="profile-card last-card">
        <div class="card-title">
          <Clock3 :size="20" />
          <div>
            <h2>运行上次配置</h2>
            <p>恢复设备、目标、模式、脚本、参数和 server 路径</p>
          </div>
        </div>
        <template v-if="lastProfile">
          <strong>{{ lastProfile.name }}</strong>
          <span>{{ lastProfile.deviceSerial }} · {{ lastProfile.mode }} · {{ lastProfile.target }}</span>
          <small>{{ lastProfile.scriptName }} · {{ formatDate(lastProfile.lastRunAt) }}</small>
          <button class="primary" type="button" :disabled="busy !== ''" @click="emit('run', lastProfile)">
            <Play :size="17" /> 一键运行上次配置
          </button>
        </template>
        <div v-else class="empty">成功运行并保存一份配置后，这里会出现快捷入口。</div>
      </article>

      <article class="profile-card save-card">
        <div class="card-title">
          <Save :size="20" />
          <div>
            <h2>保存当前配置</h2>
            <p>保存脚本快照，之后即使源脚本改变也能复用</p>
          </div>
        </div>
        <label>
          <span>配置名称</span>
          <input
            :value="draftName"
            type="text"
            maxlength="80"
            placeholder="例如：微信 Attach 调试"
            @input="emit('update:draftName', ($event.target as HTMLInputElement).value)"
          />
        </label>
        <button class="secondary" type="button" :disabled="!canSave || busy !== ''" @click="emit('save')">
          <Save :size="17" /> 保存当前运行配置
        </button>
      </article>
    </div>

    <div class="profile-list-panel">
      <div class="list-header">
        <div>
          <h2>已保存配置</h2>
          <p>{{ profiles.length }} 项 · 按最近运行排序</p>
        </div>
      </div>
      <div v-if="profiles.length === 0" class="empty list-empty">暂无运行配置</div>
      <article v-for="profile in profiles" :key="profile.id" class="profile-row">
        <div class="profile-main">
          <strong>{{ profile.name }}</strong>
          <span>{{ profile.deviceSerial }} · {{ profile.mode }} / {{ profile.targetKind }} · {{ profile.target }}</span>
          <small>{{ profile.scriptName }} · server {{ profile.serverPath }}</small>
        </div>
        <div class="profile-time">
          <span>最近运行</span>
          <small>{{ formatDate(profile.lastRunAt) }}</small>
        </div>
        <div class="profile-actions">
          <button type="button" title="载入并编辑" @click="emit('load', profile)"><Upload :size="16" /></button>
          <button class="strong" type="button" title="运行配置" :disabled="busy !== ''" @click="emit('run', profile)"><Play :size="16" /></button>
          <button class="danger" type="button" title="删除" @click="emit('delete', profile)"><Trash2 :size="16" /></button>
        </div>
      </article>
    </div>
  </section>
</template>

<style scoped>
.profiles-workspace { padding: 18px 22px 28px; display: grid; gap: 16px; overflow: auto; }
.quick-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.profile-card, .profile-list-panel { background: #fff; border: 1px solid #d8e0e5; border-radius: 10px; padding: 18px; box-shadow: 0 2px 8px rgba(24, 39, 47, .04); }
.profile-card { display: grid; align-content: start; gap: 12px; }
.last-card { background: linear-gradient(135deg, #f3fbf8, #fff); border-color: #b8dfd2; }
.card-title { display: flex; gap: 11px; color: #137c68; }
h2 { margin: 0; font-size: 17px; color: #18282f; }
p { margin: 3px 0 0; color: #6c7d85; font-size: 12px; }
.profile-card > strong { font-size: 17px; color: #17382f; }
.profile-card > span, .profile-card > small { color: #52666e; overflow-wrap: anywhere; }
label { display: grid; gap: 6px; color: #40545d; font-size: 12px; }
input { border: 1px solid #cbd6dc; border-radius: 7px; padding: 10px 11px; font: inherit; outline: none; }
input:focus { border-color: #168a73; box-shadow: 0 0 0 3px rgba(22,138,115,.1); }
button { border: 1px solid #cbd6dc; border-radius: 7px; background: #fff; color: #31464f; min-height: 36px; padding: 0 13px; display: inline-flex; align-items: center; justify-content: center; gap: 7px; cursor: pointer; }
button:disabled { opacity: .5; cursor: not-allowed; }
button.primary, button.strong { background: #168a73; color: #fff; border-color: #168a73; }
button.secondary { color: #137c68; border-color: #8fc9b9; }
.list-header { padding-bottom: 13px; border-bottom: 1px solid #e5eaed; }
.profile-row { min-height: 78px; display: grid; grid-template-columns: minmax(0, 1fr) 190px auto; align-items: center; gap: 18px; border-bottom: 1px solid #edf1f3; padding: 12px 0; }
.profile-row:last-child { border-bottom: 0; }
.profile-main { min-width: 0; display: grid; gap: 4px; }
.profile-main strong { color: #1c3038; }
.profile-main span, .profile-main small { color: #60727a; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.profile-time { display: grid; gap: 4px; color: #73838a; font-size: 12px; }
.profile-actions { display: flex; gap: 7px; }
.profile-actions button { min-width: 36px; width: 36px; padding: 0; }
.profile-actions button.danger { color: #b54141; }
.empty { color: #829198; font-size: 13px; padding: 12px 0; }
.list-empty { text-align: center; padding: 35px; }
@media (max-width: 860px) {
  .quick-grid { grid-template-columns: 1fr; }
  .profile-row { grid-template-columns: 1fr auto; }
  .profile-time { display: none; }
}
</style>
