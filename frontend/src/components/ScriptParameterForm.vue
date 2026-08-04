<script setup lang="ts">
import { SlidersHorizontal } from "@lucide/vue";
import type { ScriptMetadata, ScriptParameter } from "../lib/api";

defineProps<{
  metadata: ScriptMetadata | null;
  values: Record<string, string | number | boolean>;
  loading: boolean;
  error: string;
}>();

const emit = defineEmits<{ "update-value": [name: string, value: string | number | boolean] }>();

function update(parameter: ScriptParameter, event: Event) {
  const input = event.target as HTMLInputElement | HTMLSelectElement;
  if (parameter.type === "boolean") {
    emit("update-value", parameter.name, (input as HTMLInputElement).checked);
  } else if (parameter.type === "number") {
    emit("update-value", parameter.name, input.value === "" ? "" : Number(input.value));
  } else {
    emit("update-value", parameter.name, input.value);
  }
}
</script>

<template>
  <div class="parameter-panel" :class="{ error: Boolean(error) }">
    <div class="parameter-title">
      <SlidersHorizontal :size="17" />
      <div>
        <strong>{{ metadata?.name || "脚本参数" }}</strong>
        <span v-if="loading">正在读取 @frida-gui 元数据…</span>
        <span v-else-if="error">{{ error }}</span>
        <span v-else-if="metadata?.description">{{ metadata.description }}</span>
        <span v-else-if="metadata?.parameters.length">由脚本元数据自动生成，运行时安全写入 JS 字面量</span>
        <span v-else>脚本未声明 @frida-gui 参数，无需手动修改源码</span>
      </div>
    </div>

    <div v-if="metadata?.parameters.length" class="parameter-grid">
      <label v-for="parameter in metadata.parameters" :key="parameter.name" :class="{ toggle: parameter.type === 'boolean' }">
        <span>
          {{ parameter.label || parameter.name }}
          <em v-if="parameter.required">必填</em>
        </span>
        <select
          v-if="parameter.type === 'select'"
          :value="values[parameter.name] ?? ''"
          @change="update(parameter, $event)"
        >
          <option v-if="!parameter.required" value="">请选择</option>
          <option v-for="option in parameter.options" :key="option" :value="option">{{ option }}</option>
        </select>
        <input
          v-else-if="parameter.type === 'boolean'"
          type="checkbox"
          :checked="Boolean(values[parameter.name])"
          @change="update(parameter, $event)"
        />
        <input
          v-else
          :type="parameter.type === 'number' ? 'number' : 'text'"
          :value="values[parameter.name] ?? ''"
          :placeholder="parameter.placeholder || parameter.name"
          @input="update(parameter, $event)"
        />
        <small v-if="parameter.description" :title="parameter.description">{{ parameter.description }}</small>
      </label>
    </div>
  </div>
</template>

<style scoped>
.parameter-panel { border: 1px solid #d7e0e4; border-radius: 8px; background: #f8fafb; padding: 11px 12px; display: grid; gap: 11px; }
.parameter-panel.error { border-color: #edc2ba; background: #fff8f6; }
.parameter-title { display: flex; gap: 8px; color: #168a73; }
.parameter-title > div { display: grid; gap: 2px; }
.parameter-title strong { font-size: 13px; color: #263a43; }
.parameter-title span { color: #6d7e85; font-size: 11px; }
.parameter-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap: 9px; }
label { min-width: 0; display: grid; align-content: start; gap: 5px; color: #485d65; font-size: 11px; }
label > span { display: flex; gap: 6px; align-items: center; }
em { color: #b04b3d; font-style: normal; font-size: 10px; }
input:not([type="checkbox"]), select { width: 100%; min-height: 34px; border: 1px solid #cad5da; border-radius: 6px; padding: 0 9px; font: inherit; background: #fff; box-sizing: border-box; }
label.toggle { grid-template-columns: 1fr auto; align-items: center; border: 1px solid #d9e2e6; border-radius: 6px; padding: 8px 9px; background: #fff; }
label.toggle input { width: 17px; height: 17px; accent-color: #168a73; }
small { color: #75868d; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
