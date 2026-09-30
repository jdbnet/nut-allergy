<script setup>
import { ref } from "vue";
import { api } from "../api";

const password = ref("");
const error = ref("");
const busy = ref(false);

async function submit() {
  error.value = "";
  busy.value = true;
  try {
    await api("/api/login", { method: "POST", body: { password: password.value } });
    location.assign("/");
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <form class="panel grid max-w-md gap-3 p-6" @submit.prevent="submit">
    <h2 class="text-3xl">Unlock</h2>
    <p v-if="error" class="text-[var(--bad)]">{{ error }}</p>
    <input v-model="password" class="field" type="password" autocomplete="current-password" required />
    <button class="btn" :disabled="busy">Enter</button>
  </form>
</template>
