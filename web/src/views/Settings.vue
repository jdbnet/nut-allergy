<script setup>
import { computed, onMounted, ref } from "vue";
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
const webhookFormat = ref("teams");
const webhookTemplates = ref([]);
const noticeAlerts = ref("");
const smtpTestResult = ref(null);
const webhookTestResult = ref(null);
const webhookRowResults = ref({});

const emailPreview = computed(() => {
  if (!alerts.value?.smtp) return "";
  return "Subject: UPS alert test: Rack A (test)\nRack A (test) is running on battery (sample notification)";
});

onMounted(async () => {
  settings.value = await api("/api/settings");
  providers.value = await api("/api/dns-providers");
  seconds.value = settings.value.timeout_seconds;
  if (settings.value.tls_mode) mode.value = settings.value.tls_mode;
  alerts.value = await api("/api/settings/alerts");
  webhookTemplates.value = await api("/api/settings/webhooks/templates");
});

function templateLabel(id) {
  return webhookTemplates.value.find((t) => t.id === id)?.label || id;
}

function templateHelp(id) {
  return webhookTemplates.value.find((t) => t.id === id)?.help || "";
}

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
  smtpTestResult.value = null;
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
  smtpTestResult.value = null;
  try {
    const res = await api("/api/settings/alerts/test-email", {
      method: "POST",
      body: {
        smtp: alerts.value.smtp,
        smtp_password: smtpPassword.value,
      },
    });
    smtpTestResult.value = res;
  } catch (e) {
    smtpTestResult.value = { success: false, error: e.message };
  }
}

function formatWebhookResult(res) {
  if (!res) return "";
  if (res.success) {
    let s = "Success";
    if (res.status_code) s += ` (HTTP ${res.status_code})`;
    if (res.message) s = res.message;
    if (res.response) s += `\n${res.response}`;
    return s;
  }
  let s = res.error || "Failed";
  if (res.status_code) s += ` (HTTP ${res.status_code})`;
  if (res.response) s += `\n${res.response}`;
  return s;
}

async function testDraftWebhook() {
  error.value = "";
  webhookTestResult.value = null;
  if (!webhookURL.value.trim()) {
    webhookTestResult.value = { success: false, error: "Enter a webhook URL first." };
    return;
  }
  try {
    webhookTestResult.value = await api("/api/settings/webhooks/test", {
      method: "POST",
      body: { url: webhookURL.value.trim(), format: webhookFormat.value, event: "test" },
    });
  } catch (e) {
    webhookTestResult.value = { success: false, error: e.message };
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
    webhookTestResult.value = null;
    alerts.value = await api("/api/settings/alerts");
    noticeAlerts.value = "Webhook added.";
  } catch (e) {
    error.value = e.message;
  }
}

async function updateWebhookFormat(hook, format) {
  error.value = "";
  try {
    await api(`/api/settings/webhooks/${hook.id}`, { method: "PATCH", body: { format } });
    alerts.value = await api("/api/settings/alerts");
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
    delete webhookRowResults.value[hook.id];
  } catch (e) {
    error.value = e.message;
  }
}

async function testWebhook(hook, format) {
  error.value = "";
  webhookRowResults.value[hook.id] = null;
  try {
    webhookRowResults.value[hook.id] = await api(`/api/settings/webhooks/${hook.id}/test`, {
      method: "POST",
      body: { format, event: "test" },
    });
  } catch (e) {
    webhookRowResults.value[hook.id] = { success: false, error: e.message };
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
      <p class="mono text-xs whitespace-pre-wrap text-[var(--muted)]">{{ emailPreview }}</p>
      <div class="flex flex-wrap gap-2">
        <button class="btn" type="submit">Save alert settings</button>
        <button class="btn ghost" type="button" @click="testEmail">Send test email</button>
      </div>
      <p
        v-if="smtpTestResult"
        class="mono text-xs whitespace-pre-wrap"
        :class="smtpTestResult.success ? 'text-[var(--good)]' : 'text-[var(--bad)]'"
      >
        {{ formatWebhookResult(smtpTestResult) }}
      </p>

      <h3 class="text-xl font-semibold">Webhooks</h3>
      <ul v-if="alerts.webhooks.length" class="grid gap-3">
        <li v-for="hook in alerts.webhooks" :key="hook.id" class="grid gap-2 border border-[var(--line)] p-3">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <p class="mono text-sm">{{ hook.id }}</p>
            <p class="text-xs text-[var(--muted)]">{{ hook.url_set ? "URL saved" : "No URL" }}</p>
          </div>
          <select
            class="field"
            :value="hook.format"
            @change="updateWebhookFormat(hook, $event.target.value)"
          >
            <option v-for="t in webhookTemplates" :key="t.id" :value="t.id">{{ t.label }}</option>
          </select>
          <div class="flex flex-wrap gap-2">
            <button class="btn ghost text-sm" type="button" @click="toggleWebhook(hook, !hook.enabled)">
              {{ hook.enabled ? "Disable" : "Enable" }}
            </button>
            <button class="btn ghost text-sm" type="button" @click="testWebhook(hook, hook.format)">Send test</button>
            <button class="btn danger text-sm" type="button" @click="removeWebhook(hook)">Remove</button>
          </div>
          <p
            v-if="webhookRowResults[hook.id]"
            class="mono text-xs whitespace-pre-wrap"
            :class="webhookRowResults[hook.id].success ? 'text-[var(--good)]' : 'text-[var(--bad)]'"
          >
            {{ formatWebhookResult(webhookRowResults[hook.id]) }}
          </p>
        </li>
      </ul>
      <p v-else class="text-sm text-[var(--muted)]">No webhooks yet.</p>

      <h4 class="font-semibold">Add webhook</h4>
      <select v-model="webhookFormat" class="field">
        <option v-for="t in webhookTemplates" :key="t.id" :value="t.id">{{ t.label }}</option>
      </select>
      <p v-if="templateHelp(webhookFormat)" class="text-xs text-[var(--muted)]">{{ templateHelp(webhookFormat) }}</p>
      <p v-else-if="webhookFormat === 'teams'" class="text-xs text-[var(--muted)]">
        Teams channel → Workflows → “Post to a channel when a webhook request is received” → copy the HTTP POST URL.
      </p>
      <input v-model="webhookURL" class="field mono" placeholder="Webhook URL" />
      <div class="flex flex-wrap gap-2">
        <button class="btn ghost" type="button" @click="testDraftWebhook">Send test</button>
        <button class="btn" type="button" @click="addWebhook">Add webhook</button>
      </div>
      <p
        v-if="webhookTestResult"
        class="mono text-xs whitespace-pre-wrap"
        :class="webhookTestResult.success ? 'text-[var(--good)]' : 'text-[var(--bad)]'"
      >
        {{ formatWebhookResult(webhookTestResult) }}
      </p>
    </form>
  </div>
</template>
