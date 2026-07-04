import React from 'react';
import ReactDOM from 'react-dom/client';
import { HashRouter } from 'react-router-dom';
import 'antd/dist/reset.css';
import '@/styles/page-shell.css';
import '@/styles/app.css';

import { App } from './App';
import { I18nProvider } from './i18n';
import { ThemeProvider } from './theme';

const root = document.getElementById('root');

if (root) {
  ReactDOM.createRoot(root).render(
    <React.StrictMode>
      <ThemeProvider>
        <I18nProvider>
          <HashRouter>
            <App />
          </HashRouter>
        </I18nProvider>
      </ThemeProvider>
    </React.StrictMode>,
  );
}
