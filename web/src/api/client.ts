import createClient from 'openapi-fetch';
import { useEffect, useState } from 'react';
import type { components, paths } from './schema.gen';

// cookie сессии HttpOnly, браузер шлёт её сам, поэтому credentials
export const api = createClient<paths>({ baseUrl: '/api', credentials: 'include' });

api.use({
  onResponse({ request, response }) {
    if (response.status === 401 && !new URL(request.url).pathname.endsWith('/login')) {
      window.dispatchEvent(new Event('session-expired'));
    }
  },
});

// история нужна и банку, и клиенту в архиве; перечитываем при каждой новой версии заявки
export function useHistory(id: string, version: number, revision = 0) {
  const [history, setHistory] = useState<components['schemas']['ApplicationEvent'][]>([]);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  useEffect(() => {
    let active = true;
    api
      .GET('/applications/{id}/history', { params: { path: { id } } })
      .then(({ data }) => {
        if (!active) return;
        if (data) {
          setHistory(data);
          setState('ready');
        } else setState('error');
      })
      .catch(() => {
        if (active) setState('error');
      });
    return () => {
      active = false;
    };
  }, [id, version, revision]);
  return { history, state };
}
