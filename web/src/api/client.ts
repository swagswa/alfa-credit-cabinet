import createClient from 'openapi-fetch';
import type { paths } from './schema.gen';

// cookie сессии HttpOnly, браузер шлёт её сам, поэтому credentials
export const api = createClient<paths>({ baseUrl: '/api', credentials: 'include' });

api.use({
  onResponse({ request, response }) {
    if (response.status === 401 && !new URL(request.url).pathname.endsWith('/login')) {
      window.dispatchEvent(new Event('session-expired'));
    }
  },
});
