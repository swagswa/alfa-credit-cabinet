import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import '@alfalab/core-components-themes/corp.css';
import { App } from './App';
import { MotionProvider } from './motion';
// свои стили после компонентов, иначе их css перебивает наш при равной специфичности
import './app.css';
import './ui/tokens.css';
import './ui/workspace.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <MotionProvider><App /></MotionProvider>
    </BrowserRouter>
  </StrictMode>,
);
