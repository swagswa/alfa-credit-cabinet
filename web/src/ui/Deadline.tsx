import { Typography } from '@alfalab/core-components/typography';
import { useEffect, useState } from 'react';
import type { components } from '../api/schema.gen';

type App = components['schemas']['ApplicationView'];
type DeadlineInfo = { at: string; label: string; expired: string };

function deadlineFor(card: App): DeadlineInfo | null {
  switch (card.status) {
    case 'need_info':
      return {
        at: card.info_request?.due_at ?? card.due_at ?? '',
        label: 'Ответить банку',
        expired: 'Срок ответа истёк. Ожидаем обновление статуса от банка.',
      };
    case 'approved':
    case 'counter_offer':
      return {
        at: card.offer?.expires_at ?? card.due_at ?? '',
        label: 'Принять предложение',
        expired: 'Срок принятия предложения истёк. Ожидаем обновление статуса от банка.',
      };
    case 'accepted':
    case 'signing':
      return {
        at: card.due_at ?? '',
        label: 'Подписать документы',
        expired: 'Срок подписания истёк. Ожидаем обновление статуса от банка.',
      };
    case 'submitted':
    case 'checking':
    case 'scoring':
    case 'manual_review':
      return {
        at: card.due_at ?? '',
        label: 'Срок рассмотрения',
        expired: 'Срок рассмотрения вышел. Банк продолжает работу с заявкой.',
      };
    default:
      return null;
  }
}

export function ApplicationDeadline({ card }: { card: App }) {
  const deadline = deadlineFor(card);
  return deadline && Number.isFinite(Date.parse(deadline.at)) ? (
    <Deadline key={deadline.at + deadline.label} {...deadline} />
  ) : null;
}

export function Deadline({ at, label, expired }: DeadlineInfo) {
  const end = Date.parse(at);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!Number.isFinite(end) || end <= Date.now()) return;
    const tick = () => {
      const time = Date.now();
      setNow(time);
      if (time >= end) clearInterval(timer);
    };
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [end]);
  if (!Number.isFinite(end)) return null;
  const seconds = Math.max(0, Math.ceil((end - now) / 1000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remaining = `${hours ? `${hours}:` : ''}${hours ? String(minutes).padStart(2, '0') : minutes}:${String(seconds % 60).padStart(2, '0')}`;
  return (
    <aside className={`application-deadline${seconds === 0 ? ' application-deadline-expired' : ''}`} aria-label={label}>
      <div>
        <strong>{label}</strong>
        <time dateTime={at}>До {new Date(at).toLocaleString('ru-RU')}</time>
      </div>
      {seconds > 0 ? (
        <span className="application-deadline-count">
          Осталось <b>{remaining}</b>
        </span>
      ) : (
        <Typography.Text tag="p" view="primary-small" role="status">
          {expired}
        </Typography.Text>
      )}
    </aside>
  );
}
