<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { ClipboardCopy, Trash2 } from "@lucide/vue";
import { api } from "../api";
import { presence } from "../presence";

const fleet = ref(null);
const error = ref("");
const command = ref("");
let timer;

const lamps = {
  online: "var(--good)",
  on_battery: "var(--warn)",
  low_battery: "var(--bad)",
  unreachable: "var(--muted)",
  unknown: "var(--muted)",
};

function label(state) {
  return (
    {
      online: "Mains",
      on_battery: "Battery",
      low_battery: "Low battery",
      unreachable: "Unreachable",
      unknown: "Unknown",
    }[state] || state
  );
}

function cells(percent) {
  const n = Math.max(0, Math.min(10, Math.round((percent || 0) / 10)));
  return Array.from({ length: 10 }, (_, i) => i < n);
}

function countdown(agent) {
  if (!agent.on_battery) return "On mains";
  if (agent.shutdown) return agent.reason === "low battery" ? "Shutting down, low battery" : "Shutting down";
  if (!agent.deadline) return agent.reason;
  const left = Math.max(0, Math.round((new Date(agent.deadline) - Date.now()) / 1000));
  const m = Math.floor(left / 60);
  const s = left % 60;
  return `${m}m ${String(s).padStart(2, "0")}s`;
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
  <div v-if="fleet" class="grid gap-8 lg:grid-cols-[1.4fr_1fr]">
    <section>
      <div class="mb-3 flex items-baseline justify-between">
        <h2 class="text-3xl">Rack</h2>
        <p class="mono text-sm text-[var(--muted)]">{{ fleet.ups.length }} supplies</p>
      </div>
      <p v-if="fleet.ups.length === 0" class="text-[var(--muted)]">No UPSs yet. Add one to start polling.</p>
      <div class="grid gap-3">
        <router-link v-for="u in fleet.ups" :key="u.id" :to="`/ups/${u.id}`" class="panel block p-4">
          <div class="flex items-start justify-between gap-3">
            <div>
              <div class="flex items-center gap-2">
                <span class="lamp" :style="{ color: lamps[u.state], background: lamps[u.state] }"></span>
                <h3 class="display text-2xl">{{ u.name }}</h3>
              </div>
              <p class="text-[var(--muted)]">{{ u.description }}</p>
            </div>
            <div class="text-right">
              <p class="mono text-sm">{{ label(u.state) }}</p>
              <p class="mono text-xs text-[var(--muted)]">{{ u.host }}</p>
            </div>
          </div>
          <div class="mt-4 grid gap-3 sm:grid-cols-2">
            <div>
              <p class="mono text-xs text-[var(--muted)]">Battery {{ u.charge_percent ?? "—" }}%</p>
              <div class="cells mt-1">
                <span v-for="(on, i) in cells(u.charge_percent)" :key="i" class="cell" :class="{ on }"></span>
              </div>
            </div>
            <div>
              <p class="mono text-xs text-[var(--muted)]">Load {{ u.load_percent ?? "—" }}%</p>
              <div class="mt-2 h-2 bg-[var(--panel-2)]">
                <div class="h-2 bg-[var(--copper)]" :style="{ width: `${Math.min(u.load_percent || 0, 100)}%` }"></div>
              </div>
              <p class="mono mt-2 text-xs text-[var(--muted)]">
                {{ u.input_voltage ?? "—" }} V · {{ u.minutes_remaining ?? "—" }} min
              </p>
            </div>
          </div>
          <p v-if="u.last_error" class="mono mt-3 text-xs text-[var(--bad)]">{{ u.last_error }}</p>
        </router-link>
      </div>
    </section>
    <section>
      <div class="mb-3 flex items-baseline justify-between">
        <h2 class="text-3xl">Agents</h2>
        <button class="btn" type="button" @click="install"><ClipboardCopy :size="15" :stroke-width="1.75" /> Copy install line</button>
      </div>
      <p v-if="command" class="mono mb-3 text-xs break-all text-[var(--copper)]">{{ command }}</p>
      <p v-if="fleet.agents.length === 0" class="text-[var(--muted)]">No agents have enrolled.</p>
      <div class="grid gap-3">
        <article v-for="a in fleet.agents" :key="a.id" class="panel p-4">
          <div class="flex items-start justify-between gap-3">
            <router-link :to="`/agents/${a.id}`" class="min-w-0 text-inherit no-underline">
              <div class="flex items-center gap-2">
                <span class="lamp" :style="{ color: presence(a.last_seen).online ? 'var(--good)' : 'var(--muted)', background: presence(a.last_seen).online ? 'var(--good)' : 'var(--muted)' }"></span>
                <h3 class="display text-2xl">{{ a.hostname }}</h3>
              </div>
            </router-link>
            <button v-if="!presence(a.last_seen).online" class="btn danger" type="button" @click="removeAgent(a)">
              <Trash2 :size="14" :stroke-width="1.75" /> Remove
            </button>
          </div>
          <p class="mono mt-2 text-xs text-[var(--muted)]">
            {{ presence(a.last_seen).label }}<span v-if="presence(a.last_seen).when"> · {{ presence(a.last_seen).when }}</span>
          </p>
          <p class="mt-1 text-sm">{{ a.ups_names.length ? a.ups_names.join(" · ") : "No supplies bound" }}</p>
          <p class="mono mt-2 text-sm text-[var(--copper)]">{{ countdown(a) }}</p>
        </article>
      </div>
    </section>
  </div>
</template>
