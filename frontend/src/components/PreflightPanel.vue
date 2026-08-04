<script setup lang="ts">
import { AlertTriangle, CheckCircle2, RefreshCw, Server } from "@lucide/vue";
import type { PreflightResult } from "../lib/api";

defineProps<{
  result: PreflightResult | null;
  checking: boolean;
  repairing: boolean;
  autoCheckBeforeInject: boolean;
}>();
const emit = defineEmits<{ check: []; repair: []; toggleAutoCheck: [] }>();
</script>

<template>
  <div class="preflight" :class="{ ready: result?.ready, failed: result && !result.ready }">
    <div class="preflight-head">
      <div>
        <strong>注入前环境检查</strong>
        <span v-if="result">{{ result.ready ? "全部通过，可以注入" : "发现阻塞项，请按建议修复" }}</span>
        <span v-else>{{ autoCheckBeforeInject ? "每次注入前自动检查环境" : "手动检查模式，不会阻塞注入" }}</span>
      </div>
      <div class="actions">
        <label class="auto-check" title="控制是否在每次注入前自动运行环境检查">
          <input
            type="checkbox"
            :checked="autoCheckBeforeInject"
            :disabled="checking || repairing"
            @change="emit('toggleAutoCheck')"
          />
          <span>注入前自动检查</span>
        </label>
        <button type="button" :disabled="checking || repairing" @click="emit('check')">
          <RefreshCw :size="15" :class="{ spin: checking }" /> {{ checking ? "检查中" : "重新检查" }}
        </button>
        <button class="repair" type="button" :disabled="checking || repairing" @click="emit('repair')">
          <Server :size="15" :class="{ spin: repairing }" /> {{ repairing ? "正在修复" : "一键启动/重启 server" }}
        </button>
      </div>
    </div>
    <div v-if="result" class="check-grid">
      <div
        v-for="check in result.checks"
        :key="check.id"
        class="check"
        :class="check.status"
        :title="[check.message, check.action].filter(Boolean).join('\n')"
      >
        <CheckCircle2 v-if="check.status === 'pass'" :size="16" />
        <AlertTriangle v-else :size="16" />
        <div>
          <strong>{{ check.label }}</strong>
          <span>{{ check.message }}</span>
          <small v-if="check.status !== 'pass' && check.action">{{ check.action }}</small>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.preflight { border: 1px solid #d7e0e4; border-radius: 8px; background: #f8fafb; padding: 12px; display: grid; gap: 11px; }
.preflight.ready { background: #f0faf6; border-color: #a9d8c8; }
.preflight.failed { background: #fff9f2; border-color: #edcfaa; }
.preflight-head { display: flex; align-items: center; justify-content: space-between; gap: 14px; }
.preflight-head > div:first-child { display: grid; gap: 3px; }
.preflight-head strong { color: #263a43; font-size: 13px; }
.preflight-head span { color: #6d7e85; font-size: 12px; }
.actions { display: flex; gap: 7px; align-items: center; flex-wrap: wrap; }
.auto-check { display: inline-flex; align-items: center; gap: 6px; min-height: 31px; padding: 0 4px; color: #52656d; font-size: 11px; white-space: nowrap; cursor: pointer; }
.auto-check input { width: 15px; height: 15px; margin: 0; accent-color: #168a73; }
.auto-check:has(input:disabled) { cursor: not-allowed; opacity: .6; }
button { min-height: 31px; border: 1px solid #c7d3d9; border-radius: 6px; background: #fff; color: #40545d; padding: 0 10px; display: inline-flex; align-items: center; gap: 6px; cursor: pointer; }
button.repair { color: #fff; background: #168a73; border-color: #168a73; }
button:disabled { opacity: .5; cursor: not-allowed; }
.check-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(210px, 1fr)); gap: 7px; }
.check { min-width: 0; display: flex; align-items: flex-start; gap: 7px; border: 1px solid #dde5e8; background: #fff; border-radius: 6px; padding: 8px; color: #60727a; }
.check.pass { color: #148269; border-color: #c1e1d5; }
.check.fail { color: #b24a38; border-color: #efd1c8; }
.check > div { min-width: 0; display: grid; gap: 2px; }
.check strong { color: #31464f; font-size: 12px; }
.check span, .check small { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-size: 11px; }
.check small { color: #926b39; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (max-width: 760px) { .preflight-head { align-items: stretch; flex-direction: column; } .actions { flex-wrap: wrap; } }
</style>
