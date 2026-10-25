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
  ['admin', 'Администратор', 'только просмотр'],
];
const demoPassword = 'demo12345';

export function Login({ onLogin }: { onLogin: (session: Session) => void }) {
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
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
      <p className="eyebrow">Альфа-Хак · Прототип</p>
      <Typography.Title tag="h1">Вход в кредитный кабинет</Typography.Title>
      <p className="hint">Выберите финансирование для бизнеса и следите за заявкой в одном кабинете.</p>
      <section className="demo-guide" aria-label="Демо-учётки">
        <p>
          Демо-учётки, пароль у всех {demoPassword}. Страницы у компаний одинаковые, отличаются данные и результат
          рассмотрения.
        </p>
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
      {error && <p role="alert">{error}</p>}
      <Button type="submit" view="primary" loading={busy} disabled={!login.trim() || !password}>
        Войти
      </Button>
    </form>
  );
}
