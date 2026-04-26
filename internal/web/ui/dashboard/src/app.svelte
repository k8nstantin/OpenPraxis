<script lang="ts">
  let name = $state('Svelte');
</script>

<main>
  <h1>Hello, {name}!</h1>
  <p>OpenPraxis Dashboard v2 — bootstrap shell.</p>
  <p class="hint">
    This is the empty Svelte scaffold. Real tabs land in follow-up
    sub-product migrations (Tasks, Manifests, Runner, …).
  </p>
</main>

<style>
  main {
    max-width: 640px;
    margin: 64px auto;
    padding: 0 24px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
    color: #d8d8e6;
  }
  h1 {
    margin: 0 0 12px 0;
    font-size: 22px;
    font-weight: 600;
    color: #e4e4f0;
  }
  p {
    margin: 8px 0;
    line-height: 1.5;
    font-size: 14px;
  }
  .hint {
    color: #8a8aa0;
    font-size: 12px;
  }
</style>
