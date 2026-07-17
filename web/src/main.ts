import { createApp } from "vue";
import { createPinia } from "pinia";
import { createRouter, createWebHistory } from "vue-router";
import App from "./App.vue";
import { useAuthStore } from "./stores/auth";
import "./tailwind.css";
import "./style.css";
const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/",
      component: () => import("./views/Dashboard.vue"),
      meta: { auth: true, title: "Visão geral" },
    },
    {
      path: "/login",
      component: () => import("./views/Login.vue"),
      meta: { title: "Entrar" },
    },
    {
      path: "/setup",
      component: () => import("./views/Setup.vue"),
      meta: { title: "Configuração" },
    },
    {
      path: "/projects",
      component: () => import("./views/Projects.vue"),
      meta: { auth: true, title: "Projetos" },
    },
    {
      path: "/projects/:id",
      component: () => import("./views/ProjectDetail.vue"),
      meta: { auth: true, title: "Projeto" },
    },
    {
      path: "/projects/:projectId/applications/:applicationId",
      component: () => import("./views/ApplicationDetail.vue"),
      meta: { auth: true, title: "Aplicação" },
    },
    {
      path: "/infrastructure",
      component: () => import("./views/Infrastructure.vue"),
      meta: { auth: true, title: "Infraestrutura" },
    },
    {
      path: "/settings",
      component: () => import("./views/Settings.vue"),
      meta: { auth: true, title: "Configurações" },
    },
  ],
});
router.addRoute({
  path: "/activity",
  component: () => import("./views/Activity.vue"),
  meta: { auth: true, title: "Atividade" },
});
router.afterEach((to) => {
  document.title = `${to.meta.title || "Painel"} · StackHost`;
});
const pinia = createPinia();
const auth = useAuthStore(pinia);
router.beforeEach(async (to) => {
  if (to.path === "/setup" || to.path === "/login") return true;
  const setup = await fetch("/api/v1/setup/status")
    .then((r) => r.json())
    .catch(() => ({ needs_setup: false }));
  if (setup.needs_setup) return "/setup";
  if (!(await auth.load()))
    return `/login?redirect=${encodeURIComponent(to.fullPath)}`;
  return true;
});
createApp(App).use(pinia).use(router).mount("#app");
