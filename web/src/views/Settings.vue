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
const alerts = ref(null);
const smtpPassword = ref("");
const webhookURL = ref("");
const webhookFormat = ref("generic");
const noticeAlerts = ref("");

onMounted(async () => {
  settings.value = await api("/api/settings");
  providers.value = await api("/api/dns-providers");
  seconds.value = settings.value.timeout_seconds;
  if (settings.value.tls_mode) mode.value = settings.value.tls_mode;
  alerts.value = await api("/api/settings/alerts");
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

async function saveAlerts() {
  error.value = "";
  noticeAlerts.value = "";
  try {
    alerts.value = await api("/api/settings/alerts", {
      method: "PUT",
      body: {
        smtp: alerts.value.smtp,
        timing: alerts.value.timing,
        smtp_password: smtpPassword.value,
      },
    });
    smtpPassword.value = "";
    noticeAlerts.value = "Alert settings saved.";
  } catch (e) {
    error.value = e.message;
  }
}

async function testEmail() {
  error.value = "";
  noticeAlerts.value = "";
  try {
    await api("/api/settings/alerts/test-email", { method: "POST", body: {} });
    noticeAlerts.value = "Test email sent.";
  } catch (e) {
    error.value = e.message;
  }
}

async function addWebhook() {
  error.value = "";
  if (!webhookURL.value.trim()) return;
  try {
    await api("/api/settings/webhooks", {
      method: "POST",
      body: { url: webhookURL.value.trim(), format: webhookFormat.value },
    });
    webhookURL.value = "";
    alerts.value = await api("/api/settings/alerts");
    noticeAlerts.value = "Webhook added.";
  } catch (e) {
    error.value = e.message;
  }
}

async function toggleWebhook(hook, enabled) {
  error.value = "";
  try {
    await api(`/api/settings/webhooks/${hook.id}`, { method: "PATCH", body: { enabled } });
    alerts.value = await api("/api/settings/alerts");
  } catch (e) {
    error.value = e.message;
  }
}

async function removeWebhook(hook) {
  if (!confirm("Remove this webhook?")) return;
  error.value = "";
  try {
    await api(`/api/settings/webhooks/${hook.id}`, { method: "DELETE" });
    alerts.value = await api("/api/settings/alerts");
  } catch (e) {
    error.value = e.message;
  }
}

async function testWebhook(hook) {
  error.value = "";
  noticeAlerts.value = "";
  try {
    await api(`/api/settings/webhooks/${hook.id}/test`, { method: "POST", body: {} });
    noticeAlerts.value = "Test webhook sent.";
  } catch (e) {
    error.value = e.message;
  }
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
    <form v-if="alerts" class="panel grid gap-3 p-6" @submit.prevent="saveAlerts">
      <h2 class="text-3xl">Power alerts</h2>
      <p v-if="noticeAlerts" class="text-[var(--good)]">{{ noticeAlerts }}</p>
      <p class="text-sm text-[var(--muted)]">
        Email and webhooks fire when a UPS goes on battery, returns to mains, or reports low battery. Debounce ignores brief flaps.
      </p>
      <div class="grid gap-2 sm:grid-cols-2">
        <label class="text-sm text-[var(--muted)]">
          Debounce (seconds)
          <input v-model.number="alerts.timing.debounce_seconds" class="field mono" type="number" min="1" max="3600" required />
        </label>
        <label class="text-sm text-[var(--muted)]">
          Cooldown (seconds)
          <input v-model.number="alerts.timing.cooldown_seconds" class="field mono" type="number" min="0" max="86400" required />
        </label>
      </div>
      <label class="flex items-center gap-2 text-sm">
        <input v-model="alerts.smtp.enabled" type="checkbox" />
        Enable email (SMTP)
      </label>
      <input v-model="alerts.smtp.host" class="field mono" placeholder="SMTP host" />
      <div class="grid gap-2 sm:grid-cols-2">
        <input v-model.number="alerts.smtp.port" class="field mono" type="number" placeholder="Port" />
        <select v-model="alerts.smtp.tls_mode" class="field">
          <option value="starttls">STARTTLS</option>
          <option value="tls">Implicit TLS</option>
          <option value="none">No TLS</option>
        </select>
      </div>
      <input v-model="alerts.smtp.username" class="field" placeholder="SMTP username" />
      <input
        v-model="smtpPassword"
        class="field"
        type="password"
        :placeholder="alerts.smtp.password_set ? 'Password (leave blank to keep saved)' : 'SMTP password'"
      />
      <input v-model="alerts.smtp.from_addr" class="field mono" placeholder="From address" />
      <textarea v-model="alerts.smtp.to_addrs" class="field mono min-h-20" placeholder="Recipients (comma or newline separated)"></textarea>
      <div class="flex flex-wrap gap-2">
        <button class="btn" type="submit">Save alert settings</button>
        <button class="btn ghost" type="button" @click="testEmail">Send test email</button>
      </div>
      <h3 class="text-xl font-semibold">Webhooks</h3>
      <ul v-if="alerts.webhooks.length" class="grid gap-2">
        <li v-for="hook in alerts.webhooks" :key="hook.id" class="flex flex-wrap items-center justify-between gap-2 border border-[var(--line)] p-3">
          <div>
            <p class="mono text-sm">{{ hook.id }}</p>
            <p class="text-xs text-[var(--muted)]">{{ hook.format }} · {{ hook.url_set ? "URL saved" : "No URL" }}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button class="btn ghost text-sm" type="button" @click="toggleWebhook(hook, !hook.enabled)">
              {{ hook.enabled ? "Disable" : "Enable" }}
            </button>
            <button class="btn ghost text-sm" type="button" @click="testWebhook(hook)">Test</button>
            <button class="btn danger text-sm" type="button" @click="removeWebhook(hook)">Remove</button>
          </div>
        </li>
      </ul>
      <p v-else class="text-sm text-[var(--muted)]">No webhooks yet.</p>
      <div class="grid gap-2 sm:grid-cols-3">
        <input v-model="webhookURL" class="field mono sm:col-span-2" placeholder="https://hooks.example/..." />
        <select v-model="webhookFormat" class="field">
          <option value="generic">Generic JSON</option>
          <option value="slack">Slack</option>
          <option value="discord">Discord</option>
        </select>
      </div>
      <button class="btn ghost" type="button" @click="addWebhook">Add webhook</button>
    </form>
  </div>
</template>
