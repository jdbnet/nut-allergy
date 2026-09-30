import { createRouter, createWebHistory } from "vue-router";
import Setup from "./views/Setup.vue";
import Login from "./views/Login.vue";
import Fleet from "./views/Fleet.vue";
import Agents from "./views/Agents.vue";
import UpsForm from "./views/UpsForm.vue";
import UpsDetail from "./views/UpsDetail.vue";
import Agent from "./views/Agent.vue";
import Settings from "./views/Settings.vue";

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/setup", component: Setup },
    { path: "/login", component: Login },
    { path: "/", component: Fleet },
    { path: "/agents", component: Agents },
    { path: "/ups/new", component: UpsForm },
    { path: "/ups/:id/edit", component: UpsForm },
    { path: "/ups/:id", component: UpsDetail },
    { path: "/agents/:id", component: Agent },
    { path: "/settings", component: Settings },
  ],
});
