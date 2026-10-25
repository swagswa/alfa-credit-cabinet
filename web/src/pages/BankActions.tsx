import { MoneyField } from '../ui/MoneyField';
import { Typography } from '@alfalab/core-components/typography';
import { BottomNotice, notify } from '../BottomNotice';
import { useState, type FormEvent } from 'react';
import { Button, Checkbox, Select } from '../motion';
import { Input } from '@alfalab/core-components/input';
import { api, errorText } from '../api/client';
import { kop, options, products } from '../api/presentation';
import type { components } from '../api/schema.gen';
type App = components['schemas']['ApplicationView'];
type Outcome = components['schemas']['DecisionOutcome'];
type Code = components['schemas']['ReasonCode'];
const notices = {
  decide: 'Решение сохранено.',
  'request-info': 'Запрос отправлен клиенту.',
  'retry-checks': 'Проверки запущены.',
};
export function BankActions({
  card,
  role,
  refresh,
}: {
  card: App;
  role?: components['schemas']['Role'];
  refresh: () => void;
}) {
  const [outcome, setOutcome] = useState<Outcome>('counter_offer');
  const base = card.recommendation ?? card.offer ?? card;
  const [amount, setAmount] = useState(String(base.amount_kop / 100));
  const [term, setTerm] = useState(String(base.term_months));
  const [rate, setRate] = useState(String('rate_bp' in base ? base.rate_bp / 100 : 19));
  const [product, setProduct] = useState(base.product);
  const [comment, setComment] = useState('');
  const [infoComment, setInfoComment] = useState('');
  const [reason, setReason] = useState<Code>('R_MANUAL_RISK');
  // у причин из расчёта тексты уже с числами заявки, коду вне расчёта воркер подставит общий текст
  // из underwriter_reasons. «Смотрит специалист» после решения уже неправда, их не предлагаем
  const explained = card.reasons.filter((r) => r.text && !r.code.startsWith('R_MANUAL'));
  const [codes, setCodes] = useState<Code[]>(explained.filter((r) => r.code.startsWith('R_')).map((r) => r.code));
  const [pledge, setPledge] = useState('pledge_required' in base && !!base.pledge_required);
  const [guarantor, setGuarantor] = useState('guarantor_required' in base && !!base.guarantor_required);
  // клиент чаще всего их уже прислал, просить повторно по умолчанию не надо
  const [statement, setStatement] = useState(false);
  const [financials, setFinancials] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function act(action: 'decide' | 'request-info' | 'retry-checks', event?: FormEvent) {
    event?.preventDefault();
    if (busy) return;
    const note = (action === 'request-info' ? infoComment : comment).trim();
    if (action !== 'retry-checks' && note.length < 3) {
      setError('Добавьте комментарий от 3 символов.');
      return;
    }
    if (action === 'decide' && outcome === 'rejected' && explained.length && !codes.length) {
      setError('Отметьте хотя бы одну причину отказа.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      const terms: components['schemas']['OfferTerms'] = {
        product,
        amount_kop: kop(amount),
        term_months: Number(term),
        rate_bp: Math.round(Number(rate) * 100),
        pledge_required: pledge,
        guarantor_required: guarantor,
        msp_guarantee: false,
      };
      const result =
        action === 'decide'
          ? await api.POST('/applications/{id}/decision', {
              params: { path: { id: card.id } },
              body: {
                outcome,
                comment: note,
                reason_codes: explained.length ? codes : [reason],
                ...(outcome !== 'rejected' ? { terms } : {}),
              },
            })
          : action === 'request-info'
            ? await api.POST('/applications/{id}/request-info', {
                params: { path: { id: card.id } },
                body: {
                  comment: note,
                  documents: [
                    ...(statement ? ['statement' as const] : []),
                    ...(financials ? ['financials' as const] : []),
                  ],
                },
              })
            : await api.POST('/applications/{id}/retry-checks', { params: { path: { id: card.id } } });
      if (result.data) {
        if (action === 'request-info') setInfoComment('');
        else setComment('');
        notify(notices[action]);
        refresh();
      } else {
        setError(
          result.response.status === 409
            ? 'Заявка изменилась. Обновите карточку перед повтором.'
            : result.response.status === 403
              ? 'Для этого действия недостаточно прав.'
              : errorText(result.error, 'Действие не выполнено.'),
        );
      }
    } catch {
      setError('Нет связи с сервером. Проверьте состояние заявки перед повтором.');
    } finally {
      setBusy(false);
    }
  }
  const manager = role === 'manager' || role === 'admin';
  const underwriter = role === 'underwriter' || role === 'admin';
  return (
    <section>
      <Typography.Title tag="h2" view="small" font="system" defaultMargins>
        Действия сотрудника
      </Typography.Title>
      {error && <BottomNotice message={error} onClose={() => setError('')} />}
      {(manager || underwriter) && card.allowed_next.includes('need_info') && (
        <form onSubmit={(event) => act('request-info', event)}>
          <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
            Запросить уточнения
          </Typography.Title>
          <Checkbox
            label="Запросить выписку"
            checked={statement}
            onChange={(_, p) => setStatement(p.checked)}
            disabled={busy}
          />
          <Checkbox
            label="Запросить отчётность"
            checked={financials}
            onChange={(_, p) => setFinancials(p.checked)}
            disabled={busy}
          />
          <Input
            block
            label="Комментарий к запросу"
            required
            minLength={3}
            maxLength={1000}
            value={infoComment}
            onChange={(e) => setInfoComment(e.target.value)}
            disabled={busy}
          />
          <Button type="submit" loading={busy}>
            Отправить запрос клиенту
          </Button>
        </form>
      )}
      {/* причины в карточке пишет только решение, а с недоступным источником до него не доходит */}
      {manager && card.allowed_next.includes('checking') && card.checks?.some((c) => c.status === 'unavailable') && (
        <Button loading={busy} onClick={() => act('retry-checks')}>
          Повторить проверки
        </Button>
      )}
      {underwriter && card.allowed_next.some((s) => ['approved', 'counter_offer', 'rejected'].includes(s)) && (
        <form onSubmit={(event) => act('decide', event)}>
          <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
            Решение по заявке
          </Typography.Title>
          <Select
            block
            label="Исход решения"
            selected={outcome}
            options={[
              { key: 'approved', content: 'Одобрить' },
              { key: 'counter_offer', content: 'Предложить другие условия' },
              { key: 'rejected', content: 'Отказать' },
            ].filter((o) => card.allowed_next.includes(o.key as Outcome))}
            onChange={({ selected }) => setOutcome(selected!.key as Outcome)}
            disabled={busy}
          />
          {outcome !== 'rejected' && (
            <>
              <Select
                block
                label="Продукт решения"
                selected={product}
                options={options(products)}
                onChange={({ selected }) => setProduct(selected!.key as App['product'])}
                disabled={busy}
              />
              <MoneyField label="Сумма решения, ₽" required value={amount} onChange={setAmount} disabled={busy} />
              <Input
                block
                label="Срок решения, месяцев"
                type="number"
                min={1}
                max={60}
                required
                value={term}
                onChange={(e) => setTerm(e.target.value)}
                disabled={busy}
              />
              <Input
                block
                label="Ставка решения, %"
                type="number"
                min={1}
                max={50}
                step="0.01"
                required
                value={rate}
                onChange={(e) => setRate(e.target.value)}
                disabled={busy}
              />
              <Checkbox
                label="Требуется залог"
                checked={pledge}
                onChange={(_, p) => setPledge(p.checked)}
                disabled={busy}
              />
              <Checkbox
                label="Требуется поручительство"
                checked={guarantor}
                onChange={(_, p) => setGuarantor(p.checked)}
                disabled={busy}
              />
            </>
          )}
          {card.checks?.some((c) => c.status === 'unavailable') && (
            <Typography.Text tag="p" view="primary-small" role="note">
              Часть источников не ответила, расчёта политики нет: решение будет без него.
            </Typography.Text>
          )}
          {explained.length ? (
            <fieldset>
              <legend>Что клиент увидит в блоке «Почему принято такое решение»</legend>
              {explained.map((r, index) => (
                <Checkbox
                  key={`${r.code}-${index}`}
                  label={r.text}
                  checked={codes.includes(r.code)}
                  onChange={(_, p) =>
                    setCodes((value) => (p.checked ? [...value, r.code] : value.filter((c) => c !== r.code)))
                  }
                  disabled={busy}
                />
              ))}
              {!codes.length && (
                <Typography.Text tag="p" view="primary-small">
                  Без отмеченных причин клиент увидит только условия, без объяснения.
                </Typography.Text>
              )}
            </fieldset>
          ) : (
            <>
              <Select
                block
                label="Причина решения"
                selected={reason}
                options={[
                  { key: 'R_MANUAL_RISK', content: 'Ручная оценка риска' },
                  { key: 'R_APPROVED', content: 'Одобрение' },
                  { key: 'R_AMOUNT_DSR', content: 'Ограничение долговой нагрузки' },
                  { key: 'R_GRADE_E', content: 'Высокий риск' },
                ]}
                onChange={({ selected }) => setReason(selected!.key as Code)}
                disabled={busy}
              />
              <Typography.Text tag="p" view="primary-small">
                В расчёте нет причин для клиента: он увидит стандартный текст по выбранной причине.
              </Typography.Text>
            </>
          )}
          <Input
            block
            label="Комментарий к решению"
            required
            minLength={3}
            maxLength={2000}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            disabled={busy}
          />
          <Typography.Text tag="p" view="primary-small">
            Отклонение от рекомендации политики будет отражено в аудите.
          </Typography.Text>
          <Button type="submit" loading={busy}>
            Сохранить решение
          </Button>
        </form>
      )}
    </section>
  );
}
