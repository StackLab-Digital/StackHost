<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { Activity, FolderKanban, LayoutDashboard, Menu, Server, Settings2, X } from 'lucide-vue-next'
import { useAuthStore } from './stores/auth'
import ToastContainer from './components/ui/ToastContainer.vue'
const route = useRoute(); const auth = useAuthStore(); const open = ref(false)
const publicRoute = computed(() => route.path === '/setup' || route.path === '/login')
const links = [{ to: '/', label: 'Visão geral', icon: LayoutDashboard }, { to: '/projects', label: 'Projetos', icon: FolderKanban }, { to: '/infrastructure', label: 'Infraestrutura', icon: Server }, { to: '/activity', label: 'Atividade', icon: Activity }, { to: '/settings', label: 'Configurações', icon: Settings2 }]
const pageName = computed(() => links.find(link => link.to !== '/' && route.path.startsWith(link.to))?.label || (route.path === '/' ? 'Visão geral' : 'StackHost'))
function closeMenu() { open.value = false }
function keydown(event: KeyboardEvent) { if (event.key === 'Escape') closeMenu() }
onMounted(() => document.addEventListener('keydown', keydown)); onUnmounted(() => document.removeEventListener('keydown', keydown))
</script>
<template>
  <div v-if="publicRoute" class="public-app"><RouterView /></div>
  <div v-else class="app">
    <div v-if="open" class="mobile-overlay" @click="closeMenu" />
    <aside :class="{ open }">
      <div class="brand"><span class="mark">S</span><span>StackHost</span><button class="icon-button mobile-close" aria-label="Fechar menu" type="button" @click="closeMenu"><X :size="18" /></button></div>
      <nav><RouterLink v-for="link in links" :key="link.to" :to="link.to" @click="closeMenu"><component :is="link.icon" :size="17" aria-hidden="true" />{{ link.label }}</RouterLink></nav>
      <div class="sidebar-foot"><span class="dot" /><div><strong>Ambiente local</strong><small>Docker conectado</small></div></div>
      <RouterLink class="account-link" to="/settings" @click="closeMenu"><span class="avatar">{{ auth.user?.name?.charAt(0).toUpperCase() || 'S' }}</span><span><strong>{{ auth.user?.name || 'Administrador' }}</strong><small>{{ auth.user?.role === 'admin' ? 'Administrador' : 'Usuário' }}</small></span></RouterLink>
    </aside>
    <main>
      <header><button class="menu" type="button" aria-label="Abrir menu" @click="open=true"><Menu :size="21" /></button><div class="breadcrumb"><span>StackHost</span><b>/</b><strong>{{ pageName }}</strong></div><span class="environment-chip"><span class="dot" /> Ambiente local</span><RouterLink class="header-account" to="/settings" aria-label="Abrir configurações"><span class="avatar">{{ auth.user?.name?.charAt(0).toUpperCase() || 'S' }}</span></RouterLink></header>
      <section class="content"><RouterView /></section>
    </main>
    <ToastContainer />
  </div>
</template>
