import { InlineNotice } from '../ui/InlineNotice';
import { useSearchParams } from 'react-router';
import { Button } from '../motion';
import { useState, type FormEvent } from 'react';
import { Typography } from '@alfalab/core-components/typography';
import { Input } from '@alfalab/core-components/input';
import { api } from '../api/client';
import type { Session } from '../App';

// жюри не должно запоминать логины: каждая учётка ведёт к своему исходу
const personas = [
  ['coffee', 'ООО «Тёплое зерно»', 'кофейня, одобрение'],
  ['logistics', 'ИП Лебедев С. Н.', 'грузоперевозки, другие условия'],
  ['it', 'ООО «Кодовая база»', 'запрос документов и решение специалиста'],
  ['build', 'ООО «Стройснаб»', 'отказ с причинами'],
  ['unavailable', 'ООО «Северный путь»', 'реестр не отвечает'],
  ['auto', 'ООО «Мотор-Сервис»', 'отказ из-за просрочки по кредиту'],
  ['manager', 'Менеджер', 'очередь и карточки заявок'],
  ['underwriter', 'Андеррайтер', 'кредитные решения'],
  ['admin', 'Администратор', 'просмотр и пульт сбоев'],
];
const demoPassword = 'demo12345';
// api возвращает сюда из Альфа ID с причиной в ?error=
const alfaErrors: Record<string, string> = {
  alfa_id_denied: 'Вход через Альфа ID отменён. Попробуйте снова или войдите демо-учёткой.',
  alfa_id_unknown: 'В Альфа ID вы есть, а в кредитном кабинете нет. Войдите демо-учёткой.',
  alfa_id_expired: 'Вход через Альфа ID устарел. Нажмите кнопку ещё раз.',
  too_many_attempts: 'Слишком много попыток. Попробуйте позже.',
};

export function Login({ onLogin }: { onLogin: (session: Session) => void }) {
  const [search] = useSearchParams();
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(() => alfaErrors[search.get('error') ?? ''] ?? '');
  async function enter(user: string, secret: string) {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const { data, response } = await api.POST('/login', { body: { login: user, password: secret } });
      if (data) onLogin(data);
      else
        setError(
          response.status === 401
            ? 'Неверный логин или пароль'
            : response.status === 429
              ? 'Слишком много попыток. Попробуйте позже.'
              : 'Не удалось войти. Попробуйте ещё раз.',
        );
    } catch {
      setError('Нет связи с сервером. Попробуйте ещё раз.');
    } finally {
      setBusy(false);
    }
  }
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void enter(login.trim(), password);
  }
  return (
    <form className="login-form" onSubmit={submit}>
      <Typography.Text tag="p" view="primary-small" className="eyebrow">
        Альфа-Хак · Прототип
      </Typography.Text>
      <Typography.Title tag="h1">Вход в кредитный кабинет</Typography.Title>
      {search.get('expired') === '1' && (
        <Typography.Text tag="p" view="primary-small" role="status">
          Сессия завершена. Войдите снова, чтобы продолжить с открытой страницы.
        </Typography.Text>
      )}
      <Typography.Text tag="p" view="primary-small" className="hint">
        Выберите финансирование для бизнеса и следите за заявкой в одном кабинете.
      </Typography.Text>
      <section className="demo-guide" aria-label="Демо-учётки">
        <Typography.Text tag="p" view="primary-small">
          Демо-учётки, пароль у всех {demoPassword}. Страницы у компаний одинаковые, отличаются данные и результат
          рассмотрения.
        </Typography.Text>
        <div className="personas">
          {personas.map(([user, name, outcome]) => (
            <button key={user} type="button" disabled={busy} onClick={() => void enter(user, demoPassword)}>
              <strong>{name}</strong>
              <small>
                {user}: {outcome}
              </small>
            </button>
          ))}
        </div>
        <Button view="secondary" block href="/api/auth/alfa-id/start" disabled={busy}>
          Войти через Альфа ID
        </Button>
      </section>
      <Input
        block
        label="Логин"
        name="username"
        autoComplete="username"
        value={login}
        onChange={(event) => setLogin(event.target.value)}
        required
        disabled={busy}
      />
      <Input
        block
        label="Пароль"
        name="password"
        type="password"
        autoComplete="current-password"
        value={password}
        onChange={(event) => setPassword(event.target.value)}
        required
        disabled={busy}
      />
      {error && <InlineNotice>{error}</InlineNotice>}
      <Button type="submit" view="primary" loading={busy} disabled={!login.trim() || !password}>
        Войти
      </Button>
    </form>
  );
}
