<script setup>
import { onMounted, ref } from "vue";
import { api } from "../api";

const settings = ref(null);
const providers = ref([]);
const seconds = ref(1800);
const mode = ref("upload");
const provider = ref("cloudflare");
const token = ref("");
const email = ref("");
const certPem = ref("");
const keyPem = ref("");
const error = ref("");
const notice = ref("");
const busy = ref(false);
const command = ref("");

onMounted(async () => {
  settings.value = await api("/api/settings");
  providers.value = await api("/api/dns-providers");
  seconds.value = settings.value.timeout_seconds;
  if (settings.value.tls_mode) mode.value = settings.value.tls_mode;
});

async function saveTimeout() {
  error.value = "";
  try {
    settings.value = await api("/api/settings", { method: "PATCH", body: { seconds: Number(seconds.value) } });
    notice.value = "Timeout saved.";
  } catch (e) {
    error.value = e.message;
  }
}

function readFile(event, target) {
  const file = event.target.files?.[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = () => {
    if (target === "cert") certPem.value = String(reader.result);
    else keyPem.value = String(reader.result);
  };
  reader.readAsText(file);
}

async function saveCert() {
  error.value = "";
  notice.value = "";
  busy.value = true;
  try {
    const body =
      mode.value === "upload"
        ? { mode: "upload", cert_pem: certPem.value, key_pem: keyPem.value }
        : { mode: "lego", provider: provider.value, token: token.value, email: email.value };
    await api("/api/settings/certificate", { method: "POST", body });
    settings.value = await api("/api/settings");
    notice.value = "Certificate installed.";
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

async function install() {
  const res = await api("/api/enroll-tokens", { method: "POST", body: {} });
  command.value = res.command;
  await navigator.clipboard.writeText(res.command);
}
</script>

<template>
  <div class="grid max-w-xl gap-6">
    <p v-if="error" class="text-[var(--bad)]">{{ error }}</p>
    <p v-if="notice" class="text-[var(--good)]">{{ notice }}</p>
    <form class="panel grid gap-3 p-6" @submit.prevent="saveTimeout">
      <h2 class="text-3xl">On-battery wait</h2>
      <p class="text-sm text-[var(--muted)]">Agents use this unless they have an override. {{ settings?.hostname }}</p>
      <input v-model.number="seconds" class="field mono" type="number" min="1" max="86400" required />
      <button class="btn">Save</button>
    </form>
    <form class="panel grid gap-3 p-6" @submit.prevent="saveCert">
      <h2 class="text-3xl">Certificate</h2>
      <p class="text-sm text-[var(--muted)]">Current mode: {{ settings?.tls_mode || "none" }}. DNS-01 stays outbound.</p>
      <div class="flex gap-2">
        <button type="button" class="btn" :class="{ ghost: mode !== 'lego' }" @click="mode = 'lego'">Let's Encrypt</button>
        <button type="button" class="btn" :class="{ ghost: mode !== 'upload' }" @click="mode = 'upload'">Upload</button>
      </div>
      <template v-if="mode === 'lego'">
        <select v-model="provider" class="field">
          <option v-for="p in providers" :key="p.id" :value="p.id">{{ p.label }}</option>
        </select>
        <input v-model="token" class="field mono" placeholder="DNS API token, blank reuses the saved one" />
        <input v-model="email" class="field" type="email" placeholder="Email, blank reuses the saved one" />
      </template>
      <template v-else>
        <input class="field" type="file" @change="readFile($event, 'cert')" />
        <input class="field" type="file" @change="readFile($event, 'key')" />
      </template>
      <button class="btn" :disabled="busy">{{ busy ? "Working…" : "Replace certificate" }}</button>
    </form>
    <section class="panel grid gap-3 p-6">
      <h2 class="text-3xl">Install an agent</h2>
      <button class="btn" type="button" @click="install">Copy one-line install</button>
      <p v-if="command" class="mono text-xs break-all">{{ command }}</p>
    </section>
  </div>
</template>
