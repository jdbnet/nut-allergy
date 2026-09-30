<script setup>
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "../api";

const route = useRoute();
const router = useRouter();
const error = ref("");
const busy = ref(false);
const form = ref({
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

const editing = route.path.endsWith("/edit");

onMounted(async () => {
  if (!editing) return;
  const u = await api(`/api/ups/${route.params.id}`);
  form.value = { ...form.value, ...u, auth_password: "", priv_password: "" };
});

async function save() {
  error.value = "";
  busy.value = true;
  try {
    const body = {
      name: form.value.name,
      description: form.value.description,
      host: form.value.host,
      sec_level: form.value.sec_level,
      username: form.value.username,
      auth_protocol: form.value.auth_protocol,
      auth_password: form.value.auth_password,
      priv_protocol: form.value.priv_protocol,
      priv_password: form.value.priv_password,
    };
    if (editing) await api(`/api/ups/${route.params.id}`, { method: "PUT", body });
    else await api("/api/ups", { method: "POST", body });
    router.push("/");
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

async function remove() {
  if (!confirm(`Delete ${form.value.name}?`)) return;
  await api(`/api/ups/${route.params.id}`, { method: "DELETE" });
  router.push("/");
}
</script>

<template>
  <form class="panel grid max-w-xl gap-3 p-6" @submit.prevent="save">
    <h2 class="text-3xl">{{ editing ? "Edit UPS" : "Add UPS" }}</h2>
    <p v-if="error" class="text-[var(--bad)]">{{ error }}</p>
    <input v-model="form.name" class="field" placeholder="Name" required />
    <input v-model="form.description" class="field" placeholder="Description" />
    <input v-model="form.host" class="field mono" placeholder="SNMP host or address" required />
    <div class="grid grid-cols-2 gap-3">
      <select v-model="form.sec_level" class="field">
        <option value="authNoPriv">authNoPriv</option>
        <option value="authPriv">authPriv</option>
      </select>
      <select v-model="form.auth_protocol" class="field">
        <option>MD5</option><option>SHA</option><option>SHA224</option><option>SHA256</option><option>SHA384</option><option>SHA512</option>
      </select>
    </div>
    <input v-model="form.username" class="field" placeholder="SNMPv3 user" required />
    <input v-model="form.auth_password" class="field" type="password" :placeholder="editing ? 'Auth password (blank keeps current)' : 'Auth password'" :required="!editing" />
    <template v-if="form.sec_level === 'authPriv'">
      <select v-model="form.priv_protocol" class="field">
        <option>DES</option><option>AES</option><option>AES192</option><option>AES256</option>
      </select>
      <input v-model="form.priv_password" class="field" type="password" :placeholder="editing ? 'Privacy password (blank keeps current)' : 'Privacy password'" :required="!editing" />
    </template>
    <div class="flex gap-2">
      <button class="btn" :disabled="busy">Save</button>
      <button v-if="editing" class="btn ghost" type="button" @click="remove">Delete</button>
    </div>
  </form>
</template>
