<script setup>
defineProps({ name: String, size: { type: Number, default: 16 } })
</script>
<template>
  <span
    class="symbol"
    aria-hidden="true"
    :style="{
      '--symbol': `url('/symbols/${name}.svg')`,
      width: `${size}px`,
      height: `${size}px`,
    }"
  ></span>
</template>
