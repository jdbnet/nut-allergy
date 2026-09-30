<script setup>
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { Cpu, LogOut, Monitor, Moon, Plus, Server, Settings, Sun } from "@lucide/vue";
import { api } from "./api";
import { cycleTheme, themeMode } from "./theme";

const router = useRouter();
const ready = ref(false);
const authed = ref(false);
const setup = ref(null);
const theme = ref(themeMode());
const version = ref("");
const update = ref(null);

function versionLabel(v) {
  if (!v || v === "dev") return "dev build";
  return v.startsWith("v") ? v : `v${v}`;
}

let refreshing = false;
async function refresh() {
  if (refreshing) return;
  refreshing = true;
  try {
  const hash = new URLSearchParams(location.hash.replace(/^#/, ""));
  const cont = hash.get("continue");
  if (cont) {
    await api("/api/setup/continue", { method: "POST", body: { token: cont } });
    history.replaceState(null, "", location.pathname + location.search);
  }
  const session = await api("/api/session");
  authed.value = session.authenticated;
  setup.value = session.setup;
  const path = router.currentRoute.value.path;
  if (!session.setup.complete) {
    if (path !== "/setup") router.replace("/setup");
  } else if (!session.authenticated && path !== "/login") {
    router.replace("/login");
  } else if (session.authenticated && (path === "/login" || path === "/setup")) {
    router.replace("/");
  }
  ready.value = true;
  } finally {
    refreshing = false;
  }
}

function toggleTheme() {
  theme.value = cycleTheme();
}

const themeIcon = computed(() => (theme.value === "dark" ? Moon : theme.value === "light" ? Sun : Monitor));
const themeLabel = computed(() => (theme.value === "auto" ? "Auto" : theme.value === "dark" ? "Dark" : "Light"));

async function logout() {
  await api("/api/logout", { method: "POST", body: {} });
  authed.value = false;
  router.push("/login");
}

async function loadVersion() {
  try {
    const info = await api("/api/version");
    version.value = info.version;
    update.value = info.update;
  } catch {
    version.value = "";
  }
}

onMounted(() => {
  refresh();
  loadVersion();
  setInterval(loadVersion, 30 * 60 * 1000);
});
</script>

<template>
  <div v-if="!ready" class="p-10 text-[var(--muted)]">Opening the panel…</div>
  <div v-else class="mx-auto flex min-h-screen max-w-6xl flex-col px-5 py-6">
    <header class="mb-8 flex flex-wrap items-end justify-between gap-x-6 gap-y-3 border-b border-[var(--line)] pb-4">
      <div class="flex items-center gap-3">
        <img src="/favicon.png" alt="" width="44" height="44" class="h-11 w-11" />
        <div>
          <p class="mono text-xs tracking-[0.22em] text-[var(--copper)] uppercase">Local power</p>
          <h1 class="text-4xl leading-none">NUT Allergy</h1>
        </div>
      </div>
      <nav class="flex flex-wrap items-center gap-1">
        <template v-if="authed && setup?.complete">
          <router-link class="nav-link" to="/"><Server :size="16" :stroke-width="1.75" /> Rack</router-link>
          <router-link class="nav-link section" to="/agents"><Cpu :size="16" :stroke-width="1.75" /> Agents</router-link>
          <router-link class="nav-link" to="/ups/new"><Plus :size="16" :stroke-width="1.75" /> Add UPS</router-link>
          <router-link class="nav-link" to="/settings"><Settings :size="16" :stroke-width="1.75" /> Settings</router-link>
        </template>
        <button class="nav-link" type="button" @click="toggleTheme">
          <component :is="themeIcon" :size="16" :stroke-width="1.75" /> {{ themeLabel }}
        </button>
        <button v-if="authed && setup?.complete" class="nav-link" type="button" @click="logout">
          <LogOut :size="16" :stroke-width="1.75" /> Log out
        </button>
      </nav>
    </header>
    <div class="flex-1">
      <router-view />
    </div>
    <footer v-if="version" class="mt-10 flex flex-wrap items-baseline justify-between gap-3 border-t border-[var(--line)] pt-4">
      <span class="mono text-xs text-[var(--muted)]">{{ versionLabel(version) }}</span>
      <a v-if="update" class="mono text-xs text-[var(--copper)]" :href="update.url" target="_blank" rel="noopener">{{ versionLabel(update.version) }} is available</a>
    </footer>
  </div>
</template>
