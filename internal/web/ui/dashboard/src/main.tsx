import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import './styles.css';
import './components/ui/ui.css';

const rootEl = document.getElementById('app');
if (!rootEl) throw new Error('#app not found in index.html');

createRoot(rootEl).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
