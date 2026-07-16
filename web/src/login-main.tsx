import React from 'react';
import { createRoot } from 'react-dom/client';
import { I18nProvider } from './i18n/I18nProvider';
import { LoginPage } from './pages/LoginPage';
import 'antd/dist/reset.css';
import './styles/base.css';
import './styles/themes.css';

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <I18nProvider>
      <LoginPage />
    </I18nProvider>
  </React.StrictMode>,
);
