import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router';
import { api } from '../api/client';
import { sources } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { Button } from '../motion';
import { PageHeading, SectionHeading } from '../ui/Workspace';

type Fault = components['schemas']['SourceFault'];
type Check = components['schemas']['StreamEvent'] & { at: Date };

const modes: [Fault['mode'], string][] = [
  ['ok', 'Работает'],
  ['http_503', 'Лежит'],
  ['timeout', 'Не отвечает'],
  ['flaky', 'Моргает'],
  ['not_found', 'Не знает компанию'],
];
const outcomes = { ok: 'ответил', not_found: 'не нашёл компанию', unavailable: 'так и не ответил' };
const time = new Intl.DateTimeFormat('ru-RU', { timeStyle: 'medium' });

function step(c: Check) {
  if (c.check_status) return outcomes[c.check_status];
  return `попытка ${c.attempt} из ${c.attempts} не удалась${c.error ? `: ${c.error}` : ''}`;
}

export function Faults() {
  const [faults, setFaults] = useState<Fault[]>([]);
  const [checks, setChecks] = useState<Check[]>([]);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const load = useCallback(
    () =>
      api
        .GET('/admin/faults')
        .then(({ data }) => {
          if (data) setFaults(data);
          setError(data ? '' : 'Мок источников не отвечает, пульт недоступен.');
        })
        .catch(() => setError('Нет связи с сервером.')),
    [],
  );
  useEffect(() => {
    void load();
    // ход проверок по всем заявкам: здесь видно, как сбой отыгрывает процесс
    const stream = new EventSource('/api/events');
    stream.addEventListener('check', (event) => {
      const check = { ...(JSON.parse(event.data) as Check), at: new Date() };
      setChecks((list) => [check, ...list].slice(0, 30));
      // flaky тратит попытки, счётчик на пульте должен идти вместе с ними
      if (!check.check_status) void load();
    });
    return () => stream.close();
  }, [load]);
  async function set(source: Fault['source'], mode: Fault['mode']) {
    setBusy(source + mode);
    try {
      const { response } = await api.PUT('/admin/faults', { body: { source, mode } });
      if (!response.ok) setError('Не удалось переключить, мок источников не отвечает.');
      await load();
    } catch {
      setError('Нет связи с сервером.');
    } finally {
      setBusy('');
    }
  }
  return (
    <div className="dashboard">
      <PageHeading
        title="Пульт сбоев"
        description="Источники проверок на стенде это моки. Сбой действует на все новые запросы, пока его не снимут."
      />
      {error && <p role="alert">{error}</p>}
      <ul className="faults">
        {faults.map((fault) => (
          <li key={fault.source}>
            <strong>{sources[fault.source]}</strong>
            <div role="group" aria-label={sources[fault.source]}>
              {modes.map(([mode, label]) => (
                <Button
                  key={mode}
                  size={32}
                  view={fault.mode === mode ? 'primary' : 'secondary'}
                  aria-pressed={fault.mode === mode}
                  loading={busy === fault.source + mode}
                  onClick={() => void set(fault.source, mode)}
                >
                  {label}
                </Button>
              ))}
            </div>
            {fault.left !== undefined && <small>упадёт ещё запросов: {fault.left}</small>}
          </li>
        ))}
      </ul>
      <p>
        Как показать: включите «Лежит» у реестра и подайте заявку под любой компанией. Проверка повторится пять раз,
        заявка уйдёт специалисту. Снимите сбой, менеджер в карточке нажмёт «Повторить проверки». «Моргает» роняет три
        запроса, проверка проходит с четвёртой попытки.
      </p>
      <section aria-labelledby="faults-log">
        <SectionHeading title={<span id="faults-log">Ход проверок</span>} />
        {checks.length ? (
          <ol className="faults-log">
            {checks.map((c) => (
              <li key={`${c.at.getTime()} ${c.application_id} ${c.source} ${c.attempt}`}>
                <time>{time.format(c.at)}</time>
                <Link to={`/bank/applications/${c.application_id}`}>заявка {c.application_id.slice(-6)}</Link>
                <span>
                  {c.source && sources[c.source]}: {step(c)}
                </span>
              </li>
            ))}
          </ol>
        ) : (
          <p>Пока тихо. Запросы в источники появятся здесь, как только какая-нибудь заявка уйдёт на проверку.</p>
        )}
      </section>
    </div>
  );
}
