<script setup lang="ts">
defineProps<{
  services: string[];
  images: string[];
  ports: string[];
  volumes?: string[];
  networks?: string[];
  empty?: boolean;
}>();
</script>
<template>
  <aside class="source-summary">
    <h3>Resumo detectado</h3>
    <p v-if="empty" class="muted">
      Valide o Compose para visualizar os serviços detectados.
    </p>
    <template v-else>
      <p>
        <span>Serviços</span><strong>{{ services.length }}</strong>
      </p>
      <p>
        <span>Imagens</span><strong>{{ images.length }}</strong>
      </p>
      <p>
        <span>Portas</span><strong>{{ ports.length }}</strong>
      </p>
      <p v-if="volumes?.length">
        <span>Volumes</span><strong>{{ volumes.length }}</strong>
      </p>
      <p v-if="networks?.length">
        <span>Redes</span><strong>{{ networks.length }}</strong>
      </p>
    </template>
  </aside>
</template>
