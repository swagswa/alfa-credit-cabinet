import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import '@alfalab/core-components-themes/corp.css';
import { App } from './App';
import { MotionProvider } from './motion';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <MotionProvider><App /></MotionProvider>
    </BrowserRouter>
  </StrictMode>,
);

import './app.css';

import './ui/tokens.css';
import './ui/workspace.css';
