<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { api } from "../api";

const fleet = ref(null);
const error = ref("");
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

async function load() {
  try {
    fleet.value = await api("/api/fleet");
    error.value = "";
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
    <div class="mb-3 flex items-baseline justify-between">
      <h2 class="text-3xl">Rack</h2>
      <p class="mono text-sm text-[var(--muted)]">{{ fleet.ups.length }} supplies</p>
    </div>
    <p v-if="fleet.ups.length === 0" class="text-[var(--muted)]">No UPSs yet. Add one to start polling.</p>
    <div class="grid gap-3 lg:grid-cols-2">
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
</template>
