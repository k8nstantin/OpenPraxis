import App from './app.svelte';
import { mount } from 'svelte';

const target = document.getElementById('app');
if (!target) {
  throw new Error('mount target #app not found in index.html');
}
target.textContent = '';

const app = mount(App, { target });

export default app;
