<script setup>
import { onMounted, ref } from "vue";
import { api } from "../api";

const setup = ref(null);
const error = ref("");
const busy = ref(false);
const password = ref("");
const hostname = ref("");
const mode = ref("lego");
const providers = ref([]);
const provider = ref("cloudflare");
const token = ref("");
const email = ref("");
const certPem = ref("");
const keyPem = ref("");
const seconds = ref(1800);
const ups = ref({
  name: "",
  description: "",
  host: "",
  sec_level: "authNoPriv",
  username: "",
  auth_protocol: "SHA",
  auth_password: "",
  priv_protocol: "AES",
  priv_password: "",
});

async function load() {
  setup.value = await api("/api/setup");
  providers.value = await api("/api/dns-providers");
  if (setup.value.hostname) hostname.value = setup.value.hostname;
  if (setup.value.timeout_seconds) seconds.value = setup.value.timeout_seconds;
}

function step() {
  const s = setup.value;
  if (!s) return "load";
  if (!s.has_password) return "password";
  if (!s.hostname) return "hostname";
  if (!s.has_cert) return "certificate";
  if (!s.timeout_seconds) return "timeout";
  return "ups";
}

async function run(fn) {
  error.value = "";
  busy.value = true;
  try {
    await fn();
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

function savePassword() {
  run(() => api("/api/setup/password", { method: "POST", body: { password: password.value } }));
}
function saveHost() {
  run(() => api("/api/setup/hostname", { method: "POST", body: { hostname: hostname.value } }));
}
async function saveCert() {
  error.value = "";
  busy.value = true;
  try {
    const body =
      mode.value === "upload"
        ? { mode: "upload", cert_pem: certPem.value, key_pem: keyPem.value }
        : { mode: "lego", provider: provider.value, token: token.value, email: email.value };
    const res = await api("/api/setup/certificate", { method: "POST", body });
    if (res.https_url && res.continue_token) {
      const next = new URL(res.https_url);
      if (next.origin !== location.origin) {
        location.assign(`${res.https_url}#continue=${res.continue_token}`);
        return;
      }
      await api("/api/setup/continue", { method: "POST", body: { token: res.continue_token } });
    }
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
function saveTimeout() {
  run(() => api("/api/setup/timeout", { method: "POST", body: { seconds: Number(seconds.value) } }));
}
async function finish(withUPS) {
  error.value = "";
  busy.value = true;
  try {
    if (withUPS) await api("/api/setup/ups", { method: "POST", body: ups.value });
    await api("/api/setup/finish", { method: "POST", body: {} });
    location.assign("/");
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
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

onMounted(load);
</script>

<template>
  <section class="panel max-w-xl p-6">
    <p class="mono text-xs tracking-[0.18em] text-[var(--muted)] uppercase">First run</p>
    <h2 class="mb-4 text-3xl">Wire the panel</h2>
    <p v-if="error" class="mb-4 text-[var(--bad)]">{{ error }}</p>

    <form v-if="step() === 'password'" class="grid gap-3" @submit.prevent="savePassword">
      <p>Choose the admin password for this server. It is not printed to the log.</p>
      <input v-model="password" class="field" type="password" autocomplete="new-password" minlength="8" required />
      <button class="btn" :disabled="busy">Save password</button>
    </form>

    <form v-else-if="step() === 'hostname'" class="grid gap-3" @submit.prevent="saveHost">
      <p>DNS name agents and browsers will use. It must resolve to this machine on the LAN.</p>
      <input v-model="hostname" class="field mono" placeholder="ups.example.com" required />
      <button class="btn" :disabled="busy">Save hostname</button>
    </form>

    <form v-else-if="step() === 'certificate'" class="grid gap-3" @submit.prevent="saveCert">
      <p>The certificate is real. Let's Encrypt uses DNS-01 only, so this host never accepts an inbound challenge.</p>
      <div class="flex gap-2">
        <button type="button" class="btn" :class="{ ghost: mode !== 'lego' }" @click="mode = 'lego'">Let's Encrypt</button>
        <button type="button" class="btn" :class="{ ghost: mode !== 'upload' }" @click="mode = 'upload'">Upload</button>
      </div>
      <template v-if="mode === 'lego'">
        <label class="grid gap-1">DNS provider
          <select v-model="provider" class="field">
            <option v-for="p in providers" :key="p.id" :value="p.id">{{ p.label }}</option>
          </select>
        </label>
        <input v-model="token" class="field mono" placeholder="DNS API token" required />
        <input v-model="email" class="field" type="email" placeholder="Certificate contact email" required />
      </template>
      <template v-else>
        <label class="grid gap-1">Certificate PEM
          <input class="field" type="file" accept=".pem,.crt,.cer" @change="readFile($event, 'cert')" />
        </label>
        <label class="grid gap-1">Private key PEM
          <input class="field" type="file" accept=".pem,.key" @change="readFile($event, 'key')" />
        </label>
      </template>
      <button class="btn" :disabled="busy">{{ busy ? "Issuing…" : "Install certificate" }}</button>
    </form>

    <form v-else-if="step() === 'timeout'" class="grid gap-3" @submit.prevent="saveTimeout">
      <p>How long every bound UPS must be on battery before agents shut down. An agent can override this later.</p>
      <label class="grid gap-1">Seconds
        <input v-model.number="seconds" class="field mono" type="number" min="1" max="86400" required />
      </label>
      <button class="btn" :disabled="busy">Save timeout</button>
    </form>

    <form v-else class="grid gap-3" @submit.prevent="finish(true)">
      <p>Add the first UPS, or skip and land on an empty fleet.</p>
      <input v-model="ups.name" class="field" placeholder="Name" required />
      <input v-model="ups.description" class="field" placeholder="Description" />
      <input v-model="ups.host" class="field mono" placeholder="SNMP host" required />
      <div class="grid grid-cols-2 gap-3">
        <select v-model="ups.sec_level" class="field">
          <option value="authNoPriv">authNoPriv</option>
          <option value="authPriv">authPriv</option>
        </select>
        <select v-model="ups.auth_protocol" class="field">
          <option>MD5</option><option>SHA</option><option>SHA224</option><option>SHA256</option><option>SHA384</option><option>SHA512</option>
        </select>
      </div>
      <input v-model="ups.username" class="field" placeholder="SNMPv3 user" required />
      <input v-model="ups.auth_password" class="field" type="password" placeholder="Auth password" required />
      <template v-if="ups.sec_level === 'authPriv'">
        <select v-model="ups.priv_protocol" class="field">
          <option>DES</option><option>AES</option><option>AES192</option><option>AES256</option>
        </select>
        <input v-model="ups.priv_password" class="field" type="password" placeholder="Privacy password" required />
      </template>
      <div class="flex gap-2">
        <button class="btn" :disabled="busy">Add UPS and finish</button>
        <button class="btn ghost" type="button" :disabled="busy" @click="finish(false)">Skip</button>
      </div>
    </form>
  </section>
</template>
