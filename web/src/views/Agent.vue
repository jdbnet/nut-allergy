<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Trash2 } from "@lucide/vue";
import { api } from "../api";
import { presence } from "../presence";

const route = useRoute();
const router = useRouter();
const error = ref("");
const agent = ref(null);
const ups = ref([]);
const selected = ref({});
const override = ref("");
const useOverride = ref(false);

let timer;

async function load() {
  const [a, list] = await Promise.all([api(`/api/agents/${route.params.id}`), api("/api/ups")]);
  const first = !agent.value;
  agent.value = a;
  ups.value = list;
  if (!first) return;
  for (const id of a.ups_ids) selected.value[id] = true;
  if (a.timeout_override_seconds) {
    useOverride.value = true;
    override.value = String(a.timeout_override_seconds);
  }
}

async function remove() {
  if (!confirm(`Remove ${agent.value.hostname}? It will need to enroll again.`)) return;
  error.value = "";
  try {
    await api(`/api/agents/${route.params.id}`, { method: "DELETE" });
    router.push("/agents");
  } catch (e) {
    error.value = e.message;
  }
}

onMounted(() => {
  load().catch((e) => {
    error.value = e.message;
  });
  timer = setInterval(() => {
    if (!agent.value) return;
    api(`/api/agents/${route.params.id}`)
      .then((a) => {
        agent.value.last_seen = a.last_seen;
      })
      .catch(() => {});
  }, 5000);
});
onUnmounted(() => clearInterval(timer));

async function save() {
  error.value = "";
  try {
    const ups_ids = ups.value.filter((u) => selected.value[u.id]).map((u) => u.id);
    await api(`/api/agents/${route.params.id}`, {
      method: "PATCH",
      body: {
        ups_ids,
        timeout_override_seconds: useOverride.value ? Number(override.value) : null,
      },
    });
    router.push("/agents");
  } catch (e) {
    error.value = e.message;
  }
}
</script>

<template>
  <form v-if="agent" class="panel grid max-w-xl gap-4 p-6" @submit.prevent="save">
    <div>
      <p class="mono text-xs tracking-[0.18em] text-[var(--muted)] uppercase">Agent</p>
      <h2 class="text-4xl">{{ agent.hostname }}</h2>
      <p class="mono mt-1 text-sm text-[var(--muted)]">
        {{ presence(agent.last_seen).label }}<span v-if="presence(agent.last_seen).when"> · {{ presence(agent.last_seen).when }}</span>
      </p>
    </div>
    <p v-if="error" class="text-[var(--bad)]">{{ error }}</p>
    <fieldset class="grid gap-2">
      <legend class="mb-1">Plugged into</legend>
      <label v-for="u in ups" :key="u.id" class="flex items-center gap-2">
        <input v-model="selected[u.id]" type="checkbox" />
        <span>{{ u.name }}</span>
        <span class="mono text-xs text-[var(--muted)]">{{ u.host }}</span>
      </label>
    </fieldset>
    <label class="flex items-center gap-2">
      <input v-model="useOverride" type="checkbox" />
      Override the server timeout
    </label>
    <input v-if="useOverride" v-model="override" class="field mono" type="number" min="1" max="86400" required />
    <p v-else class="text-sm text-[var(--muted)]">Using the server wait of {{ agent.effective_timeout_seconds }} seconds.</p>
    <div class="flex flex-wrap gap-2">
      <button class="btn">Save and push on next poll</button>
      <button v-if="!presence(agent.last_seen).online" class="btn danger" type="button" @click="remove">
        <Trash2 :size="14" :stroke-width="1.75" /> Remove agent
      </button>
    </div>
  </form>
</template>
