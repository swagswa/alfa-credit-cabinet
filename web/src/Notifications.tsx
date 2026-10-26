import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { Badge } from '@alfalab/core-components/badge';
import { Popover } from '@alfalab/core-components/popover';
import { ModalMobile } from '@alfalab/core-components/modal/mobile';
import { useMatchMedia } from '@alfalab/core-components/mq';
import { Typography } from '@alfalab/core-components/typography';
import { CrossMIcon } from '@alfalab/icons-glyph/CrossMIcon';
import { PureCell } from '@alfalab/core-components/pure-cell';
import { api } from './api/client';
import type { components } from './api/schema.gen';
import { notify } from './BottomNotice';
import { Button, useMotionDisabled } from './motion';
import { Icon } from './ui/Workspace';

type Notification = components['schemas']['Notification'];

// экран подписания открывает ленту, когда код уже ушёл
export const openNotifications = () => window.dispatchEvent(new Event('open-notifications'));

const time = (value: string) =>
  new Date(value).toLocaleString('ru-RU', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });

export function NotificationBell() {
  const [mobile] = useMatchMedia('(max-width: 767px)');
  const reduced = useMotionDisabled();
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
    if (!open || mobile) return;
    const outside = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!panel.current?.contains(target) && !anchor?.contains(target)) setOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false);
        anchor?.focus();
      }
    };
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  }, [open, anchor, mobile]);

  const unread = list.filter((n) => !n.read_at).length;
  const close = () => {
    setOpen(false);
    if (!mobile) anchor?.focus();
  };
  async function markRead(n: Notification) {
    if (!n.read_at) {
      await api.POST('/notifications/{id}/read', { params: { path: { id: n.id } } }).catch(() => undefined);
      void load();
    }
  }
  async function pick(n: Notification) {
    close();
    await markRead(n);
    // код подписи нужен на той же карточке, уводить с неё незачем
    if (n.kind !== 'signing_code') void navigate(`/client/applications/${n.application_id}`);
  }
  async function readAll() {
    await api.POST('/notifications/read-all').catch(() => undefined);
    void load();
  }

  const content = (
    <div
      ref={panel}
      className="notification-list"
      role={mobile ? undefined : 'dialog'}
      aria-label={mobile ? undefined : 'Уведомления'}
      tabIndex={mobile ? undefined : -1}
    >
      <div className="notification-head">
        <div className="notification-heading">
          <Typography.Title tag="h2" view="xsmall" font="system">
            Уведомления
          </Typography.Title>
          <Typography.Text tag="p" view="secondary-small" color="secondary">
            {unread ? `Непрочитанных: ${unread}` : 'Всё прочитано'}
          </Typography.Text>
        </div>
        <Button
          className="notification-close"
          size={mobile ? 48 : 32}
          view="text"
          aria-label="Закрыть уведомления"
          onClick={close}
        >
          <CrossMIcon width={20} height={20} aria-hidden="true" />
        </Button>
      </div>
      {list.length > 0 && (
        <div className="notification-toolbar">
          <Button size={32} view="text" disabled={unread === 0} onClick={readAll}>
            Прочитать все
          </Button>
        </div>
      )}
      <div className="notification-items">
        {list.length === 0 && (
          <div className="notification-empty">
            <Icon name="bell" />
            <strong>Пока нет уведомлений</strong>
            <p>Здесь появятся решения банка, запросы документов и коды подписания.</p>
          </div>
        )}
        {list.map((n) => (
          <NotificationItem key={n.id} notification={n} onPick={() => void pick(n)} onRead={() => void markRead(n)} />
        ))}
      </div>
    </div>
  );

  return (
    <>
      <Button
        ref={setAnchor}
        className="profile-logout notification-bell"
        size={40}
        view="text"
        aria-label={unread ? `Уведомления, непрочитанных: ${unread}` : 'Уведомления'}
        aria-expanded={open}
        aria-haspopup="dialog"
        onClick={() => setOpen((v) => !v)}
      >
        <Icon name="bell" />
        {unread > 0 && <Badge className="notification-count" view="count" content={unread} height={18} />}
      </Button>
      {mobile ? (
        <ModalMobile
          open={open}
          onClose={close}
          className="notification-sheet"
          wrapperClassName="notification-sheet-wrapper"
          wrapperProps={{ 'aria-label': 'Уведомления', 'aria-modal': true }}
          contentProps={{ className: 'notification-sheet-content' }}
          backdropProps={{ opacity: 0.24 }}
          disableRestoreFocus
          transitionProps={{
            timeout: reduced ? 0 : 200,
            classNames: 'notification-sheet-motion',
            onExited: () => requestAnimationFrame(() => anchor?.focus()),
          }}
          zIndex={60}
        >
          {content}
        </ModalMobile>
      ) : (
        <Popover
          preventOverflow={false}
          open={open}
          anchorElement={(anchor?.closest('.profile-chip') as HTMLElement | null) ?? anchor}
          position="right-end"
          fallbackPlacements={['right-start', 'top-start', 'bottom-start', 'left-end']}
          offset={[0, 12]}
          popperClassName="notification-popover-surface"
          transition={{
            timeout: reduced ? 0 : 180,
            classNames: 'notification-popover-motion',
            onEntered: () => {
              if (!panel.current?.contains(document.activeElement)) panel.current?.focus();
            },
          }}
          transitionDuration={reduced ? '0ms' : '180ms'}
          zIndex={50}
        >
          {content}
        </Popover>
      )}
    </>
  );
}

function NotificationItem({
  notification: n,
  onPick,
  onRead,
}: {
  notification: Notification;
  onPick: () => void;
  onRead: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const code = n.kind === 'signing_code' ? n.text.match(/^Код\s+(\d{6})\b/)?.[1] : undefined;
  const number = n.text.match(/заявке №\s*(\d+)/)?.[1];
  async function copy() {
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      onRead();
    } catch {
      notify('Не удалось скопировать код. Выделите цифры и скопируйте вручную.', 'error');
    }
  }
  if (code)
    return (
      <article
        className={`notification notification-code${n.read_at ? '' : ' notification-new'}`}
        data-signing-application={n.application_id}
      >
        <strong>{n.title}</strong>
        <p className="notification-description">Подписание договора{number ? ` · заявка №${number}` : ''}</p>
        <div className="notification-code-row">
          <output aria-label="Код подписания">{code}</output>
          <Button view="secondary" size={40} onClick={copy}>
            {copied ? 'Скопирован' : 'Копировать'}
          </Button>
        </div>
        <p className="notification-code-hint">
          Никому не сообщайте код. Срок действия указан в блоке подписания заявки.
        </p>
        <time className="notification-time" dateTime={n.created_at}>
          {time(n.created_at)}
        </time>
        <span className="notification-copy-status" role="status">
          {copied ? 'Код скопирован' : ''}
        </span>
      </article>
    );
  return (
    <PureCell
      tag="button"
      className={n.read_at ? 'notification' : 'notification notification-new'}
      verticalPadding="compact"
      onClick={onPick}
    >
      <PureCell.Content>
        <PureCell.Main>
          <PureCell.Text titleColor="primary" view="component-primary" titleWeight={n.read_at ? 'regular' : 'bold'}>
            {n.title}
          </PureCell.Text>
          <PureCell.Text titleColor="secondary" view="primary-small">
            {n.text}
          </PureCell.Text>
          <PureCell.Text titleColor="tertiary" view="primary-small">
            <time dateTime={n.created_at}>{time(n.created_at)}</time>
          </PureCell.Text>
        </PureCell.Main>
      </PureCell.Content>
    </PureCell>
  );
}
