<script setup>
defineProps({
  compact: { type: Boolean, default: false },
  inverse: { type: Boolean, default: false },
})
</script>

<template>
  <div class="app-logo" :class="{ compact, inverse }">
    <span class="logo-mark" aria-hidden="true">
      <span></span>
      <span></span>
      <span></span>
    </span>
    <span v-if="!compact" class="logo-copy">
      <strong>AI Gateway</strong>
      <small>Management Center</small>
    </span>
  </div>
</template>

<style scoped>
.app-logo {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  color: var(--text-primary);
}

.app-logo.inverse {
  color: #fffdf9;
}

.logo-mark {
  position: relative;
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
}

.logo-mark span {
  position: absolute;
  width: 25px;
  height: 9px;
  border: 2px solid currentColor;
  border-radius: 999px;
  transform: rotate(60deg);
}

.logo-mark span:nth-child(2) {
  transform: rotate(-60deg);
}

.logo-mark span:nth-child(3) {
  transform: rotate(0deg);
}

.logo-copy {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.logo-copy strong {
  font-size: 15px;
  line-height: 1.2;
  letter-spacing: -0.01em;
}

.logo-copy small {
  color: var(--text-tertiary);
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.13em;
  text-transform: uppercase;
}

.inverse .logo-copy small {
  color: rgb(255 253 249 / 52%);
}
</style>
