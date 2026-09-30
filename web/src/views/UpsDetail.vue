<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import {
  Chart,
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  Tooltip,
  Legend,
  Filler,
  CategoryScale,
} from "chart.js";
import { api } from "../api";

Chart.register(LineController, LineElement, PointElement, LinearScale, Tooltip, Legend, Filler, CategoryScale);

const route = useRoute();
const range = ref("24h");
const data = ref(null);
const error = ref("");
let timer;
let chart;

const lamps = {
  online: "var(--good)",
  on_battery: "var(--warn)",
  low_battery: "var(--bad)",
  unreachable: "var(--muted)",
  unknown: "var(--muted)",
};

const rangeOptions = [
  { value: "1h", label: "1 hour" },
  { value: "24h", label: "24 hours" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
];

function stateLabel(state) {
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

function formatTime(iso) {
  const d = new Date(iso);
  return d.toLocaleString();
}

const hasSamples = computed(() => (data.value?.samples?.length ?? 0) > 0);

async function load() {
  error.value = "";
  try {
    data.value = await api(`/api/ups/${route.params.id}/history?range=${range.value}`);
  } catch (e) {
    error.value = e.message;
  }
}

function destroyChart() {
  if (chart) {
    chart.destroy();
    chart = null;
  }
}

function renderChart() {
  destroyChart();
  const canvas = document.getElementById("ups-metrics-chart");
  if (!canvas || !data.value?.samples?.length) return;

  const samples = data.value.samples;
  const labels = samples.map((s) => new Date(s.recorded_at));
  const load = samples.map((s) => s.load_percent ?? null);
  const charge = samples.map((s) => s.charge_percent ?? null);
  const volts = samples.map((s) => s.input_voltage ?? null);
  const runtime = samples.map((s) => s.minutes_remaining ?? null);

  const ctx = canvas.getContext("2d");
  const copper = getComputedStyle(document.documentElement).getPropertyValue("--copper").trim() || "#b8612d";
  const good = getComputedStyle(document.documentElement).getPropertyValue("--good").trim() || "#2f6a3a";
  const warn = getComputedStyle(document.documentElement).getPropertyValue("--warn").trim() || "#a15c12";
  const muted = getComputedStyle(document.documentElement).getPropertyValue("--muted").trim() || "#6e6256";

  chart = new Chart(ctx, {
    type: "line",
    data: {
      labels,
      datasets: [
        {
          label: "Load %",
          data: load,
          borderColor: copper,
          backgroundColor: "transparent",
          tension: 0.2,
          spanGaps: true,
          yAxisID: "y",
        },
        {
          label: "Battery %",
          data: charge,
          borderColor: good,
          backgroundColor: "transparent",
          tension: 0.2,
          spanGaps: true,
          yAxisID: "y",
        },
        {
          label: "Input V",
          data: volts,
          borderColor: warn,
          backgroundColor: "transparent",
          tension: 0.2,
          spanGaps: true,
          yAxisID: "y1",
        },
        {
          label: "Runtime min",
          data: runtime,
          borderColor: muted,
          backgroundColor: "transparent",
          tension: 0.2,
          spanGaps: true,
          yAxisID: "y1",
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: "index", intersect: false },
      scales: {
        x: {
          type: "category",
          ticks: {
            maxTicksLimit: 8,
            callback: (_, i) => {
              const d = labels[i];
              if (!d) return "";
              return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
            },
          },
        },
        y: {
          position: "left",
          min: 0,
          max: 100,
          title: { display: true, text: "%" },
        },
        y1: {
          position: "right",
          grid: { drawOnChartArea: false },
          title: { display: true, text: "V / min" },
        },
      },
      plugins: {
        legend: { position: "bottom" },
      },
    },
  });
}

watch([data, range], () => {
  renderChart();
});

watch(range, () => {
  load();
});

onMounted(() => {
  load();
  timer = setInterval(load, 15000);
});

onUnmounted(() => {
  clearInterval(timer);
  destroyChart();
});
</script>

<template>
  <p v-if="error" class="mb-4 text-[var(--bad)]">{{ error }}</p>
  <section v-if="data">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="flex items-center gap-2">
          <span class="lamp" :style="{ color: lamps[data.ups.state], background: lamps[data.ups.state] }"></span>
          <h2 class="display text-3xl">{{ data.ups.name }}</h2>
        </div>
        <p class="text-[var(--muted)]">{{ data.ups.description || data.ups.host }}</p>
        <p class="mono mt-1 text-sm text-[var(--muted)]">
          {{ stateLabel(data.ups.state) }} · {{ data.ups.host }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <div class="flex gap-1 rounded border border-[var(--line)] p-1">
          <button
            v-for="opt in rangeOptions"
            :key="opt.value"
            type="button"
            class="btn text-sm"
            :class="range === opt.value ? 'bg-[var(--panel-2)]' : ''"
            @click="range = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
        <router-link class="btn" :to="`/ups/${route.params.id}/edit`">Edit SNMP</router-link>
      </div>
    </div>

    <div class="panel mb-4 p-4">
      <h3 class="mb-3 text-lg font-semibold">Metrics</h3>
      <p v-if="!hasSamples" class="text-[var(--muted)]">
        No samples in this range yet. The server records a snapshot about once per minute while the UPS is reachable.
      </p>
      <div v-else class="h-72">
        <canvas id="ups-metrics-chart"></canvas>
      </div>
    </div>

    <div class="panel p-4">
      <h3 class="mb-3 text-lg font-semibold">Events</h3>
      <p v-if="data.events.length === 0" class="text-[var(--muted)]">No events in this range.</p>
      <ul v-else class="divide-y divide-[var(--line)]">
        <li v-for="ev in data.events" :key="ev.id" class="flex flex-wrap items-baseline justify-between gap-2 py-2 text-sm">
          <div>
            <p class="font-medium">{{ ev.message }}</p>
            <p v-if="ev.event_type !== 'state_change'" class="mono text-xs text-[var(--muted)]">{{ ev.event_type }}</p>
          </div>
          <div class="mono text-right text-xs text-[var(--muted)]">
            <p>{{ formatTime(ev.created_at) }}</p>
            <p v-if="ev.load_percent != null || ev.charge_percent != null">
              Load {{ ev.load_percent ?? "—" }}% · Battery {{ ev.charge_percent ?? "—" }}%
            </p>
          </div>
        </li>
      </ul>
    </div>
  </section>
</template>
