import createClient from 'openapi-fetch';
import { useEffect, useState } from 'react';
import type { components, paths } from './schema.gen';

// cookie сессии HttpOnly, браузер шлёт её сам, поэтому credentials
export const api = createClient<paths>({ baseUrl: '/api', credentials: 'include' });

// /me без сессии отвечает 204, а не 401: так первый экран не пишет ошибку в консоль
api.use({
  onResponse({ request, response }) {
    const path = new URL(request.url).pathname;
    if ((response.status === 401 && !path.endsWith('/login')) || (response.status === 204 && path.endsWith('/me'))) {
      window.dispatchEvent(new Event('session-expired'));
    }
  },
});

// ответ, пришедший после размонтирования или смены зависимостей эффекта, выкидываем;
// возвращает cleanup для useEffect
export function settle<T>(request: Promise<T>, done: (result: T) => void, fail = () => {}) {
  let active = true;
  request
    .then((result) => {
      if (active) done(result);
    })
    .catch(() => {
      if (active) fail();
    });
  return () => {
    active = false;
  };
}

// title в problem+json это текст http-статуса по-английски, клиенту показываем причины по полям,
// а без них detail: на 400 и 413 сервер пишет его по-русски для человека
export function errorText(
  error: { errors?: { message: string }[]; detail?: string; status?: number } | undefined,
  fallback: string,
) {
  const text =
    error?.errors?.map((e) => e.message).join('; ') || ([400, 413].includes(error?.status ?? 0) ? error?.detail : '');
  return text ? text[0].toUpperCase() + text.slice(1) + '.' : fallback;
}

// история нужна и банку, и клиенту в архиве; перечитываем при каждой новой версии заявки
export function useHistory(id: string, version: number, revision = 0) {
  const [history, setHistory] = useState<components['schemas']['ApplicationEvent'][]>([]);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  useEffect(
    () =>
      settle(
        api.GET('/applications/{id}/history', { params: { path: { id } } }),
        ({ data }) => {
          if (data) {
            setHistory(data);
            setState('ready');
          } else setState('error');
        },
        () => setState('error'),
      ),
    [id, version, revision],
  );
  return { history, state };
}
