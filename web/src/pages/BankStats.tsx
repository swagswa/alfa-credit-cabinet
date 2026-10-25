import { Typography } from '@alfalab/core-components/typography';
import { InlineNotice } from '../ui/InlineNotice';
import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { money } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { Button } from '../motion';
import { PageHeading, SectionHeading, StatusBadge } from '../ui/Workspace';

type Stats = components['schemas']['BankStats'];
const periods = [7, 30, 90];

function duration(sec: number) {
  if (sec < 60) return `${sec} с`;
  if (sec < 3600) return `${Math.round(sec / 60)} мин`;
  const h = Math.floor(sec / 3600),
    m = Math.round((sec % 3600) / 60);
  return m ? `${h} ч ${m} мин` : `${h} ч`;
}
const share = (part: number, total: number) => (total ? `${Math.round((part / total) * 100)}%` : '0%');

export function BankStats() {
  const [days, setDays] = useState(30);
  const [stats, setStats] = useState<Stats>();
  const [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    const load = () =>
      api
        .GET('/stats', { params: { query: { days } } })
        .then(({ data }) => {
          if (!active) return;
          if (data) {
            setStats(data);
            setError('');
          } else setError('Не удалось загрузить сводку.');
        })
        .catch(() => {
          if (active) setError('Нет связи с сервером.');
        });
    void load();
    // сводка живая: любая смена статуса её двигает
    const stream = new EventSource('/api/events');
    stream.addEventListener('status', () => void load());
    return () => {
      active = false;
      stream.close();
    };
  }, [days]);
  const top = Math.max(1, ...(stats?.by_status.map((item) => item.count) ?? []));
  return (
    <div className="dashboard bank-stats">
      <PageHeading title="Сводка" description="Заявки, поданные за выбранный период" />
      <div className="stats-periods" role="group" aria-label="Период сводки">
        {periods.map((value) => (
          <Button key={value} size={40} aria-pressed={value === days} view="secondary" onClick={() => setDays(value)}>
            {value} дн.
          </Button>
        ))}
      </div>
      {error && <InlineNotice>{error}</InlineNotice>}
      {!stats && !error && (
        <div className="dashboard-loading" role="status">
          <span />
          Считаем…
        </div>
      )}
      {stats && (
        <>
          <div className="dashboard-metrics stats-metrics">
            <div>
              <span>Подано</span>
              <strong>{stats.submitted}</strong>
            </div>
            <div>
              <span>Решено автоматически</span>
              <strong>{share(stats.auto_decided, stats.decided)}</strong>
            </div>
            <div>
              <span>Одобрено из решённых</span>
              <strong>{share(stats.approved, stats.decided)}</strong>
            </div>
            <div>
              <span>До решения, медиана</span>
              <strong>{duration(stats.decision_time.median_sec)}</strong>
            </div>
            <div>
              <span>До решения, 90% заявок</span>
              <strong>{duration(stats.decision_time.p90_sec)}</strong>
            </div>
            <div>
              <span>Выдано</span>
              <strong>{money(stats.issued_kop, true).replace(/\u00a0/g, ' ')}</strong>
            </div>
          </div>
          <section aria-labelledby="stats-status">
            <SectionHeading title={<span id="stats-status">Где сейчас заявки</span>} />
            {stats.by_status.length ? (
              <ul className="stats-statuses">
                {stats.by_status.map((item) => (
                  <li key={item.status}>
                    <StatusBadge status={item.status} />
                    <span className="stats-bar" style={{ width: `${(item.count / top) * 100}%` }} aria-hidden="true" />
                    <strong>{item.count}</strong>
                    {item.overdue > 0 && <small>просрочено {item.overdue}</small>}
                  </li>
                ))}
              </ul>
            ) : (
              <Typography.Text tag="p" view="primary-small">
                За период заявок не было.
              </Typography.Text>
            )}
          </section>
        </>
      )}
    </div>
  );
}
