import { Button, MotionDetails } from '../motion';
import { useState, type FormEvent } from 'react';
import { Typography } from '@alfalab/core-components/typography';
import { Input } from '@alfalab/core-components/input';
import { api } from '../api/client';
import type { Session } from '../App';

export function Login({ onLogin }: { onLogin: (session: Session) => void }) {
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const { data, response } = await api.POST('/login', { body: { login: login.trim(), password } });
      if (data) onLogin(data);
      else setError(response.status === 401 ? 'Неверный логин или пароль' : response.status === 429 ? 'Слишком много попыток. Попробуйте позже.' : 'Не удалось войти. Попробуйте ещё раз.');
    } catch { setError('Нет связи с сервером. Попробуйте ещё раз.'); }
    finally { setBusy(false); }
  }
  return <form className="login-form" onSubmit={submit}>
    <p className="eyebrow">Альфа-Хак · Прототип</p>
    <Typography.Title tag="h1">Вход в кредитный кабинет</Typography.Title>
    <p className="hint">Выберите финансирование для бизнеса и следите за заявкой в одном кабинете.</p>
    <MotionDetails className="demo-guide"><summary>Как посмотреть прототип</summary><p>У всех компаний одинаковые страницы. Учётные записи отличаются только демо-данными и результатом рассмотрения.</p><p>Клиенты: coffee: одобрение, logistics: другие условия, it: запрос документов, build: отказ.</p><p>Сотрудники: manager ведёт заявки, underwriter принимает кредитные решения. Пароль у всех: demo12345.</p></MotionDetails>
    <Input block label="Логин" name="username" autoComplete="username" value={login} onChange={(event) => setLogin(event.target.value)} required disabled={busy} />
    <Input block label="Пароль" name="password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required disabled={busy} />
    {error && <p role="alert">{error}</p>}
    <Button type="submit" view="primary" loading={busy} disabled={!login.trim() || !password}>Войти</Button>
  </form>;
}
