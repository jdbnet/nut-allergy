<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { ClipboardCopy, Trash2 } from "@lucide/vue";
import { api } from "../api";
import { presence } from "../presence";

const fleet = ref(null);
const error = ref("");
const command = ref("");
let timer;

function countdown(agent) {
  if (!agent.on_battery) return "On mains";
  if (agent.shutdown) return agent.reason === "low battery" ? "Shutting down, low battery" : "Shutting down";
  if (!agent.deadline) return agent.reason;
  const left = Math.max(0, Math.round((new Date(agent.deadline) - Date.now()) / 1000));
  const m = Math.floor(left / 60);
  const s = left % 60;
  return `${m}m ${String(s).padStart(2, "0")}s`;
}

function powerClass(agent) {
  if (agent.shutdown) return "text-[var(--bad)]";
  if (agent.on_battery) return "text-[var(--warn)]";
  return "";
}

function waitLabel(seconds) {
  if (!seconds) return "—";
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60} min`;
  return `${seconds}s`;
}

async function removeAgent(agent) {
  if (!confirm(`Remove ${agent.hostname}? It will need to enroll again.`)) return;
  error.value = "";
  try {
    await api(`/api/agents/${agent.id}`, { method: "DELETE" });
    await load();
  } catch (e) {
    error.value = e.message;
  }
}

async function load() {
  try {
    fleet.value = await api("/api/fleet");
    error.value = "";
  } catch (e) {
    error.value = e.message;
  }
}

async function install() {
  error.value = "";
  try {
    const res = await api("/api/enroll-tokens", { method: "POST", body: {} });
    command.value = res.command;
    await navigator.clipboard.writeText(res.command);
  } catch (e) {
    error.value = e.message;
  }
}

onMounted(() => {
  load();
  timer = setInterval(load, 5000);
});
onUnmounted(() => clearInterval(timer));
</script>

<template>
  <p v-if="error" class="mb-4 text-[var(--bad)]">{{ error }}</p>
  <section v-if="fleet">
    <div class="mb-3 flex flex-wrap items-baseline justify-between gap-3">
      <div>
        <h2 class="text-3xl">Agents</h2>
        <p class="mono text-sm text-[var(--muted)]">{{ fleet.agents.length }} enrolled</p>
      </div>
      <button class="btn" type="button" @click="install">
        <ClipboardCopy :size="15" :stroke-width="1.75" /> Copy install line
      </button>
    </div>
    <p v-if="command" class="mono mb-3 text-xs break-all text-[var(--copper)]">{{ command }}</p>
    <p v-if="fleet.agents.length === 0" class="text-[var(--muted)]">No agents have enrolled.</p>
    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      <article v-for="a in fleet.agents" :key="a.id" class="panel flex flex-col p-4">
        <div class="flex items-start justify-between gap-3">
          <router-link :to="`/agents/${a.id}`" class="min-w-0 text-inherit no-underline">
            <div class="flex items-center gap-2">
              <span
                class="lamp shrink-0"
                :style="{
                  color: presence(a.last_seen).online ? 'var(--good)' : 'var(--muted)',
                  background: presence(a.last_seen).online ? 'var(--good)' : 'var(--muted)',
                }"
              ></span>
              <h3 class="display truncate text-2xl">{{ a.hostname }}</h3>
            </div>
          </router-link>
          <span class="mono shrink-0 text-xs" :class="presence(a.last_seen).online ? 'text-[var(--good)]' : 'text-[var(--muted)]'">
            {{ presence(a.last_seen).online ? "Online" : "Offline" }}
          </span>
        </div>
        <dl class="mt-4 grid gap-1.5 text-sm">
          <div class="flex items-baseline justify-between gap-3">
            <dt class="text-[var(--muted)]">Power</dt>
            <dd class="mono text-right" :class="powerClass(a)">{{ countdown(a) }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-3">
            <dt class="text-[var(--muted)]">Last seen</dt>
            <dd class="mono text-right text-[var(--muted)]">{{ presence(a.last_seen).when || "Never" }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-3">
            <dt class="text-[var(--muted)]">Wait</dt>
            <dd class="mono text-right">{{ waitLabel(a.effective_timeout_seconds) }}</dd>
          </div>
        </dl>
        <div class="mt-4 flex flex-1 flex-wrap content-start gap-1.5">
          <span v-for="name in a.ups_names" :key="name" class="chip">{{ name }}</span>
          <span v-if="a.ups_names.length === 0" class="text-sm text-[var(--muted)]">No supplies bound</span>
        </div>
        <div class="mt-4 flex items-center justify-between gap-2">
          <router-link class="text-sm font-medium text-[var(--copper)]" :to="`/agents/${a.id}`">Configure</router-link>
          <button v-if="!presence(a.last_seen).online" class="btn danger" type="button" @click="removeAgent(a)">
            <Trash2 :size="14" :stroke-width="1.75" /> Remove
          </button>
        </div>
      </article>
    </div>
  </section>
</template>
