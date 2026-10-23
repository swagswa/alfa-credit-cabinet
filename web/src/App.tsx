import { Icon } from './ui/Workspace';
import { NoticeHost } from './BottomNotice';
import { CabinetCacheProvider } from './CabinetCache';
import { MotionNavLink as NavLink, RouteSurface } from './motion';
import { useEffect, useState } from 'react';
import { Navigate, Route, Routes } from 'react-router';
import { Button } from './motion';
import { Login } from './pages/Login';
import { ClientCabinet } from './pages/ClientCabinet';
import { BankCabinet } from './pages/BankCabinet';
import { api } from './api/client';
import type { components } from './api/schema.gen';

export type Session = components['schemas']['Me'];

export function App() {
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [leaving, setLeaving] = useState(false);
  useEffect(() => {
    const expire = () => { setSession(null); setError(''); };
    window.addEventListener('session-expired', expire);
    let active = true;
    api.GET('/me').then(({ data, response }) => {
      if (!active) return;
      if (data) setSession(data);
      else if (response.status !== 401) setError('Не удалось проверить вход. Обновите страницу.');
    }).catch(() => {
      if (active) setError('Нет связи с сервером. Обновите страницу.');
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; window.removeEventListener('session-expired', expire); };
  }, []);
  if (loading) return <p role="status">Проверяем вход...</p>;
  if (error && !session) return <p role="alert">{error}</p>;
  const home = session?.user.role === 'client' ? '/client' : '/bank';
  async function logout() {
    setLeaving(true);
    setError('');
    try {
      const { response } = await api.POST('/logout');
      if (response.ok || response.status === 401) setSession(null);
      else setError('Не удалось выйти. Попробуйте ещё раз.');
    } catch { setError('Нет связи с сервером. Попробуйте ещё раз.'); }
    finally { setLeaving(false); }
  }
  return (
    <CabinetCacheProvider key={session ? `${session.user.id}:${session.user.login}` : 'guest'}><main id="main-content"><NoticeHost /><a className="skip-link" href="#cabinet-content">Перейти к содержимому</a>

      <div className={session?'cabinet-layout':''}>
{session&&<aside className="sidebar"><div className="cabinet-title">Финансирование<br/>бизнеса</div><p className="sidebar-caption">Кредитный кабинет</p><nav aria-label="Разделы кабинета">{home==='/client'?<><NavLink end to="/client"><Icon name="overview"/>Обзор</NavLink><NavLink to="/client/applications"><Icon name="applications"/>Мои заявки</NavLink><NavLink to="/client/new"><Icon name="new"/>Новая заявка</NavLink></>:<NavLink to="/bank"><Icon name="applications"/>Очередь заявок</NavLink>}</nav><div className="sidebar-help"><strong>Всё в одном месте</strong><p>{home==='/client'?'Банк запросил документы? Ответьте прямо в карточке заявки.':'Откройте заявку, чтобы увидеть проверки, документы и доступные действия.'}</p></div><header className="profile-bar"><div className="profile-chip"><span className="profile-avatar" aria-hidden="true"><svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="8" r="4"/><path d="M4 21v-2a8 8 0 0 1 16 0v2H4Z"/></svg></span><strong title={session.user.role === 'client' ? session.companies[0]?.company.short_name ?? session.user.full_name : session.user.full_name}>{session.user.role === 'client' ? session.companies[0]?.company.short_name ?? session.user.full_name : session.user.full_name}</strong><Button className="profile-logout" size={40} view="text" aria-label="Выйти" title="Выйти" loading={leaving} onClick={logout}><svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M13 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h7M15 8l4 4-4 4m-5-4h9" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"/></svg></Button></div>{error && <p role="alert">{error}</p>}</header><small>Прототип для Альфа-Хак</small></aside>}
<div id="cabinet-content" tabIndex={-1}><RouteSurface><Routes>
        <Route path="/login" element={session ? <Navigate to={home} replace /> : <Login onLogin={setSession} />} />
        <Route path="/client/applications/:id" element={!session ? <Navigate to="/login" replace /> : home === '/client' ? <ClientCabinet key={session.user.id + session.user.login} /> : <Navigate to={home} replace />} />
        <Route path="/client/*" element={!session ? <Navigate to="/login" replace /> : home === '/client' ? <ClientCabinet key={session.user.id + session.user.login} /> : <Navigate to={home} replace />} />
        <Route path="/bank/applications/:id" element={!session ? <Navigate to="/login" replace /> : home === '/bank' ? <BankCabinet key={session.user.id + session.user.login} /> : <Navigate to={home} replace />} />
        <Route path="/bank/*" element={!session ? <Navigate to="/login" replace /> : home === '/bank' ? <BankCabinet key={session.user.id + session.user.login} /> : <Navigate to={home} replace />} />
        <Route path="*" element={<Navigate to={session ? home : '/login'} replace />} />
      </Routes></RouteSurface></div></div>
    </main></CabinetCacheProvider>
  );
}
