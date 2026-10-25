import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { Badge } from '@alfalab/core-components/badge';
import { Popover } from '@alfalab/core-components/popover';
import { PureCell } from '@alfalab/core-components/pure-cell';
import { api } from './api/client';
import type { components } from './api/schema.gen';
import { notify } from './BottomNotice';
import { Button } from './motion';

type Notification = components['schemas']['Notification'];

// экран подписания открывает ленту, когда код уже ушёл
export const openNotifications = () => window.dispatchEvent(new Event('open-notifications'));

const time = (value: string) =>
  new Date(value).toLocaleString('ru-RU', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });

export function NotificationBell() {
  const [list, setList] = useState<Notification[]>([]);
  const [open, setOpen] = useState(false);
  const [anchor, setAnchor] = useState<HTMLAnchorElement | HTMLButtonElement | null>(null);
  const panel = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const load = useCallback(
    () =>
      api
        .GET('/notifications', { params: { query: { limit: 20 } } })
        .then(({ data }) => {
          if (data) setList(data);
          return data?.[0];
        })
        .catch(() => undefined),
    [],
  );
  useEffect(() => {
    void load();
    const stream = new EventSource('/api/events');
    stream.addEventListener('notification', () => {
      void load().then((last) => last && notify(last.title));
    });
    const show = () => {
      setOpen(true);
      void load();
    };
    window.addEventListener('open-notifications', show);
    return () => {
      stream.close();
      window.removeEventListener('open-notifications', show);
    };
  }, [load]);

  // панель в портале: клик мимо неё и мимо колокольчика закрывает, как и Escape
  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!panel.current?.contains(target) && !anchor?.contains(target)) setOpen(false);
    };
    const escape = (event: KeyboardEvent) => event.key === 'Escape' && setOpen(false);
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  }, [open, anchor]);

  const unread = list.filter((n) => !n.read_at).length;
  async function pick(n: Notification) {
    setOpen(false);
    if (!n.read_at) {
      await api.POST('/notifications/{id}/read', { params: { path: { id: n.id } } }).catch(() => undefined);
      void load();
    }
    // код подписи нужен на той же карточке, уводить с неё незачем
    if (n.kind !== 'signing_code') void navigate(`/client/applications/${n.application_id}`);
  }
  async function readAll() {
    await api.POST('/notifications/read-all').catch(() => undefined);
    void load();
  }

  return (
    <>
      <Button
        ref={setAnchor}
        className="profile-logout notification-bell"
        size={40}
        view="text"
        aria-label={unread ? `Уведомления, непрочитанных: ${unread}` : 'Уведомления'}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path
            d="M18 16v-5a6 6 0 1 0-12 0v5l-2 2h16l-2-2ZM10 21h4"
            stroke="currentColor"
            strokeWidth="1.7"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
        {unread > 0 && <Badge className="notification-count" view="count" content={unread} height={18} />}
      </Button>
      <Popover
        preventOverflow={false}
        open={open}
        anchorElement={anchor}
        position="top-start"
        offset={[0, 8]}
        zIndex={50}
      >
        <div ref={panel} className="notification-list" role="dialog" aria-label="Уведомления">
          <div className="notification-head">
            <strong>Уведомления</strong>
            {unread > 0 && (
              <Button size={32} view="text" onClick={readAll}>
                Прочитать все
              </Button>
            )}
          </div>
          {list.length === 0 && <p>Пока ничего нет. Сюда придут решения банка и код подписания.</p>}
          {list.map((n) => (
            <PureCell
              key={n.id}
              tag="button"
              className={n.read_at ? 'notification' : 'notification notification-new'}
              verticalPadding="compact"
              onClick={() => void pick(n)}
            >
              <PureCell.Content>
                <PureCell.Main>
                  <PureCell.Text
                    titleColor="primary"
                    view="component-primary"
                    titleWeight={n.read_at ? 'regular' : 'bold'}
                  >
                    {n.title}
                  </PureCell.Text>
                  <PureCell.Text titleColor="secondary" view="primary-small">
                    {n.text}
                  </PureCell.Text>
                  <PureCell.Text titleColor="tertiary" view="primary-small">
                    {time(n.created_at)}
                  </PureCell.Text>
                </PureCell.Main>
              </PureCell.Content>
            </PureCell>
          ))}
        </div>
      </Popover>
    </>
  );
}
