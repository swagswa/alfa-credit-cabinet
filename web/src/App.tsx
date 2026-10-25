import { Typography } from '@alfalab/core-components/typography';
import { InlineNotice } from './ui/InlineNotice';
import { useEffect, useState, type ReactNode } from 'react';
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router';
import { Icon } from './ui/Workspace';
import { NoticeHost } from './BottomNotice';
import { NotificationBell } from './Notifications';
import { CabinetCacheProvider } from './CabinetCache';
import { Button, MotionNavLink as NavLink, RouteSurface } from './motion';
import { Login } from './pages/Login';
import { Dashboard } from './pages/Dashboard';
import { Applications } from './pages/Applications';
import { BankStats } from './pages/BankStats';
import { Faults } from './pages/Faults';
import { Profile } from './pages/Profile';
import { api, settle } from './api/client';
import type { components } from './api/schema.gen';

export type Session = components['schemas']['Me'];

export function App() {
  const location = useLocation();
  const navigate = useNavigate();
  const [expired, setExpired] = useState(false);
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [leaving, setLeaving] = useState(false);
  useEffect(() => {
    const expire = () => {
      setExpired(true);
      setSession(null);
      setError('');
    };
    window.addEventListener('session-expired', expire);
    const cancel = settle(
      api.GET('/me'),
      ({ data, response }) => {
        if (data) setSession(data);
        else if (response.status !== 204 && response.status !== 401)
          setError('Не удалось проверить вход. Обновите страницу.');
        setLoading(false);
      },
      () => {
        setError('Нет связи с сервером. Обновите страницу.');
        setLoading(false);
      },
    );
    return () => {
      cancel();
      window.removeEventListener('session-expired', expire);
    };
  }, []);
  if (loading)
    return (
      <Typography.Text tag="p" view="primary-small" role="status">
        Проверяем вход...
      </Typography.Text>
    );
  if (error && !session) return <InlineNotice>{error}</InlineNotice>;
  const client = session?.user.role === 'client';
  const admin = session?.user.role === 'admin';
  const home = client ? '/client' : '/bank';
  const name = client
    ? (session?.companies[0]?.company.short_name ?? session?.user.full_name)
    : session?.user.full_name;
  const only = (root: string, page: ReactNode) =>
    !session ? (
      <Navigate to={loginPath(location.pathname + location.search, expired)} replace />
    ) : home === root ? (
      page
    ) : (
      <Navigate to={home} replace />
    );
  async function logout() {
    setLeaving(true);
    setError('');
    try {
      const { response } = await api.POST('/logout');
      if (response.ok || response.status === 401) {
        setExpired(false);
        setSession(null);
        try {
          if (session) sessionStorage.removeItem(`credit:new:v1:${session.user.id}`);
        } catch {
          /* storage unavailable */
        }
        navigate('/login', { replace: true });
      } else setError('Не удалось выйти. Попробуйте ещё раз.');
    } catch {
      setError('Нет связи с сервером. Попробуйте ещё раз.');
    } finally {
      setLeaving(false);
    }
  }
  return (
    <CabinetCacheProvider key={session ? `${session.user.id}:${session.user.login}` : 'guest'}>
      <main id="main-content">
        <NoticeHost />
        <a className="skip-link" href="#cabinet-content">
          Перейти к содержимому
        </a>
        <div className={session ? 'cabinet-layout' : ''}>
          {session && (
            <aside className="sidebar">
              <div className="cabinet-title">
                Финансирование
                <br />
                бизнеса
              </div>
              <Typography.Text tag="p" view="primary-small" className="sidebar-caption">
                Кредитный кабинет
              </Typography.Text>
              <nav aria-label="Разделы кабинета">
                {client ? (
                  <>
                    <NavLink end to="/client">
                      <Icon name="overview" />
                      Обзор
                    </NavLink>
                    <NavLink to="/client/applications">
                      <Icon name="applications" />
                      Мои заявки
                    </NavLink>
                    <NavLink to="/client/new">
                      <Icon name="new" />
                      Новая заявка
                    </NavLink>
                    <NavLink to="/client/profile">
                      <Icon name="profile" />
                      Профиль
                    </NavLink>
                  </>
                ) : (
                  <>
                    <NavLink to="/bank" end>
                      <Icon name="applications" />
                      Очередь заявок
                    </NavLink>
                    <NavLink to="/bank/stats">
                      <Icon name="overview" />
                      Сводка
                    </NavLink>
                    {admin && (
                      <NavLink to="/bank/faults">
                        <Icon name="refresh" />
                        Пульт сбоев
                      </NavLink>
                    )}
                  </>
                )}
              </nav>
              <div className="sidebar-help">
                <strong>Всё в одном месте</strong>
                <Typography.Text tag="p" view="primary-small">
                  {client
                    ? 'Банк запросил документы? Ответьте прямо в карточке заявки.'
                    : 'Откройте заявку, чтобы увидеть проверки, документы и доступные действия.'}
                </Typography.Text>
              </div>
              <header className="profile-bar">
                <div className="profile-chip">
                  <span className="profile-avatar" aria-hidden="true">
                    <Icon name="user" />
                  </span>
                  <strong title={name}>{name}</strong>
                  {client && <NotificationBell />}
                  <Button
                    className="profile-logout"
                    size={40}
                    view="text"
                    aria-label="Выйти"
                    title="Выйти"
                    loading={leaving}
                    onClick={logout}
                  >
                    <Icon name="logout" />
                  </Button>
                </div>
                {error && <InlineNotice>{error}</InlineNotice>}
              </header>
              <small>Прототип для Альфа-Хак</small>
            </aside>
          )}
          <div id="cabinet-content" tabIndex={-1}>
            <RouteSurface>
              <Routes>
                <Route
                  path="/login"
                  element={
                    session ? (
                      <Navigate to={returnPath(location.search, home)} replace />
                    ) : (
                      <Login onLogin={setSession} />
                    )
                  }
                />
                <Route path="/client" element={only('/client', <Dashboard />)} />
                <Route
                  path="/client/new"
                  element={only(
                    '/client',
                    <Applications key={`new-application:${session?.user.id}`} createOnly userId={session?.user.id} />,
                  )}
                />
                <Route path="/client/applications/:id?" element={only('/client', <Applications />)} />
                <Route path="/client/profile" element={only('/client', <Profile />)} />
                <Route path="/bank/stats" element={only('/bank', <BankStats />)} />
                <Route
                  path="/bank/faults"
                  element={only('/bank', admin ? <Faults /> : <Navigate to="/bank" replace />)}
                />
                <Route
                  path="/bank/applications?/:id?"
                  element={only('/bank', <Applications bank role={session?.user.role} />)}
                />
                <Route path="*" element={<Navigate to={session ? home : '/login'} replace />} />
              </Routes>
            </RouteSurface>
          </div>
        </div>
      </main>
    </CabinetCacheProvider>
  );
}

function loginPath(path: string, expired: boolean) {
  return '/login?' + new URLSearchParams({ returnTo: path, ...(expired ? { expired: '1' } : {}) });
}
function returnPath(search: string, home: string) {
  const path = new URLSearchParams(search).get('returnTo') ?? '';
  return path === home || path.startsWith(home + '/') || path.startsWith(home + '?') ? path : home;
}
