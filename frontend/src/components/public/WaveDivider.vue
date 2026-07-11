<template>
  <div class="wave-divider" aria-hidden="true" />
</template>

<style scoped>
.wave-divider {
  position: absolute;
  inset-inline: 0;
  bottom: calc(var(--space-1) * -1);
  height: var(--layout-wave-height);
  overflow: hidden;
}

.wave-divider::before {
  position: absolute;
  inset-inline: calc(var(--layout-gutter) * -1);
  top: var(--space-6);
  height: calc(var(--layout-wave-height) * 2);
  border-radius: 50% 50% 0 0 / 100% 100% 0 0;
  background: var(--color-canvas);
  content: "";
}
</style>