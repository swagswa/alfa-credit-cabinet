import { Typography } from '@alfalab/core-components/typography';
import { checkSources, processLinks, type CheckProgressState } from '../api/checkProgress';
import { sources } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { Icon } from '../ui/Workspace';

type App = components['schemas']['ApplicationView'];
const clientSources = {
  egrul: 'Регистрация компании',
  fns: 'Налоговые сведения',
  bki: 'Кредитная история',
  internal: 'Данные вашего бизнеса в банке',
};

export function CheckProgress({
  card,
  bank,
  live,
  disconnected,
}: {
  card: App;
  bank: boolean;
  live: CheckProgressState;
  disconnected: boolean;
}) {
  const checking = ['submitted', 'checking'].includes(card.status);
  if (
    card.status === 'draft' ||
    (!bank && !['submitted', 'checking', 'scoring', 'manual_review', 'need_info'].includes(card.status))
  )
    return null;
  if (!bank && !checking && !card.checks?.length && (live.applicationId !== card.id || !Object.keys(live.items).length))
    return null;
  const rows = checkSources.map((source) => {
    const saved = card.checks
      ?.filter(
        (check) =>
          check.source === source && (!checking || Date.parse(check.checked_at) >= Date.parse(card.updated_at)),
      )
      .sort((a, b) => Date.parse(b.checked_at) - Date.parse(a.checked_at))[0];
    const candidate = live.applicationId === card.id ? live.items[source] : undefined;
    const event = candidate && (!checking || candidate.version >= card.version) ? candidate : undefined;
    const status = event ? event.check_status : saved?.status;
    const retrying = !status && !!event?.attempt && checking;
    const text =
      status === 'ok'
        ? 'Сведения получены'
        : status === 'unavailable'
          ? 'Источник не ответил'
          : status === 'not_found'
            ? bank
              ? 'Сведения не найдены'
              : 'Нужны уточнения'
            : retrying
              ? disconnected
                ? 'Ожидаем соединения'
                : 'Повторяем запрос'
              : checking
                ? 'Ожидаем ответа'
                : 'Результат не передан';
    return { source, status, retrying, text, event, saved };
  });
  const completed = rows.filter((row) => row.status).length;
  const links = bank ? processLinks(card.id, window.location.origin) : undefined;
  return (
    <section className="check-progress" aria-label="Ход проверок">
      <div className="check-progress-heading">
        <Typography.Title tag="h2" view="small" font="system" defaultMargins>
          Ход проверок
        </Typography.Title>
        {bank && (
          <span className="hint">
            Завершено {completed} из {checkSources.length}
          </span>
        )}
      </div>
      <Typography.Text tag="p" view="primary-small" className="hint">
        {bank
          ? 'Ответы источников и повторные попытки обновляются автоматически.'
          : 'Проверяем сведения о бизнесе. Решение по кредиту появится отдельно.'}
      </Typography.Text>
      {disconnected && (
        <Typography.Text tag="p" view="primary-small" role="status" className="check-connection">
          Связь прервалась. Восстанавливаем обновления…
        </Typography.Text>
      )}
      <ol className="check-progress-list" aria-live="polite" aria-atomic="false">
        {rows.map(({ source, status, retrying, text, event, saved }) => (
          <li
            key={source}
            className={`check-row check-${status ?? (retrying && !disconnected ? 'retrying' : 'waiting')}`}
          >
            <span className="check-indicator" aria-hidden="true">
              {status === 'ok' ? <Icon name="check" /> : status ? '!' : <span />}
            </span>
            <div className="check-row-content">
              <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
                {bank ? sources[source] : clientSources[source]}
              </Typography.Title>
              <Typography.Text tag="p" view="primary-small">
                {text}
              </Typography.Text>
              {event?.attempt && (
                <small>
                  {status ? 'Последняя попытка' : 'Попытка'} {event.attempt} из {event.attempts}
                </small>
              )}
              {bank && (event?.error || (!event && saved?.error)) && (
                <Typography.Text tag="p" view="primary-small" className="check-error">
                  {event?.error ?? saved?.error}
                </Typography.Text>
              )}
            </div>
          </li>
        ))}
      </ol>
      {rows.some((row) => row.status === 'unavailable') && !bank && (
        <Typography.Text tag="p" view="primary-small" className="check-explanation">
          Не все сведения удалось получить автоматически. Банк продолжит рассмотрение и сообщит, если понадобится ваше
          участие.
        </Typography.Text>
      )}
      {links && (
        <div className="check-process-links">
          <a href={links.workflow} target="_blank" rel="noopener noreferrer">
            Процесс в Temporal ↗
          </a>
          <a href={links.trace} target="_blank" rel="noopener noreferrer">
            Трейс в Jaeger ↗
          </a>
          <small className="hint">Jaeger доступен при запуске профиля наблюдаемости.</small>
        </div>
      )}
    </section>
  );
}
