<template>
  <NuxtPage />
  <CredentialVault :owner-token="mind.token" />
</template>

<script setup lang="ts">
import CredentialVault from './features/mind/ui/CredentialVault.vue'
import { useMind } from './features/mind/model/store'
const mind = useMind()
</script>
