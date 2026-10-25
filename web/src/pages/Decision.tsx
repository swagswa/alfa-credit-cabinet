import { openNotifications } from '../Notifications';
import { CodeInput } from '@alfalab/core-components/code-input';
import { Table } from '@alfalab/core-components/table';
import { AppLink as Link } from '../ui/AppLink';
import { Typography } from '@alfalab/core-components/typography';
import { InlineNotice } from '../ui/InlineNotice';
import { Disclosure } from '../ui/Disclosure';
import { money, rate, products } from '../api/presentation';
import { isCompleted } from '../api/dashboard';
import { repeatApplication } from '../api/repeatApplication';
import { CounterOfferComparison } from './CounterOfferComparison';
import { Value } from '../motion/Value';
import { Button, motion, useMotionDisabled } from '../motion';
import { OutcomePanel } from '../motion/OutcomePanel';
import { BottomNotice, notify } from '../BottomNotice';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { Deadline } from '../ui/Deadline';
import { Modal } from '@alfalab/core-components/modal';
import { api, errorText } from '../api/client';
import type { components } from '../api/schema.gen';
type App = components['schemas']['ApplicationView'];
export function Decision({ card, refresh }: { card: App; refresh: () => void }) {
  const reduced = useMotionDisabled();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const codeInput = useRef<React.ComponentRef<typeof CodeInput>>(null);
  const [code, setCode] = useState('');
  const [issued, setIssued] = useState<components['schemas']['SigningCode'] | null>(null);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (isCompleted(card.status) || (!issued && !card.offer)) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [card.status, card.offer, issued]);
  const codeExpired = !!issued && Date.parse(issued.expires_at) <= now;
  const contract = card.documents.find((doc) => doc.kind === 'loan_agreement');
  const offerExpired =
    !!card.offer && Date.parse(card.offer.expires_at) <= now && card.allowed_next.includes('accepted');
  async function action(kind: 'accept' | 'signing-code') {
    if (busy || (kind === 'accept' && offerExpired)) return;
    setBusy(true);
    setError('');
    try {
      const result =
        kind === 'accept'
          ? await api.POST('/applications/{id}/accept', {
              params: { path: { id: card.id } },
              body: { version: card.version },
            })
          : await api.POST('/applications/{id}/signing-code', { params: { path: { id: card.id } } });
      if (result.data) {
        if ('attempts_left' in result.data) {
          setIssued(result.data);
          setCode('');
          openNotifications();
        } else {
          notify('Условия приняты.');
          refresh();
        }
      } else
        setError(
          result.response.status === 409
            ? 'Состояние заявки изменилось. Обновите карточку и проверьте условия.'
            : errorText(result.error, 'Не удалось выполнить действие.'),
        );
    } catch {
      setError('Нет связи с сервером. Проверьте статус перед повтором.');
    } finally {
      setBusy(false);
    }
  }
  async function sign(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || codeExpired || code.length !== 6) return;
    setBusy(true);
    setError('');
    try {
      const { data, error, response } = await api.POST('/applications/{id}/sign', {
        params: { path: { id: card.id } },
        body: { code },
      });
      if (data) {
        setIssued(null);
        setCode('');
        refresh();
      } else {
        setError(errorText(error, 'Подписание не выполнено.'));
        if (response.status === 422) {
          setIssued((previous) =>
            previous && previous.attempts_left > 1 && Date.parse(previous.expires_at) > Date.now()
              ? { ...previous, attempts_left: previous.attempts_left - 1 }
              : null,
          );
          setCode('');
          codeInput.current?.reset();
        }
      }
    } catch {
      setError('Нет связи с сервером. Проверьте статус перед повтором.');
    } finally {
      setBusy(false);
    }
  }
  if (card.status === 'need_info') return null;
  if (card.status === 'expired' || card.status === 'cancelled_by_client') {
    const expired = card.status === 'expired';
    return (
      <section className="archive-panel terminal-result">
        <Typography.Title tag="h2" view="small" font="system" defaultMargins>
          {expired ? (card.offer ? 'Срок предложения истёк' : 'Срок ответа истёк') : 'Заявка отменена'}
        </Typography.Title>
        <Typography.Text tag="p" view="primary-small">
          {expired
            ? 'Работа по этой заявке завершена. Чтобы снова обратиться за финансированием, создайте новую заявку.'
            : 'Вы отменили эту заявку. Она сохранена только для просмотра.'}
        </Typography.Text>
        <Typography.Text tag="p" view="primary-small">
          <strong>{card.company.short_name}</strong> · {money(card.amount_kop)} · {card.term_months} мес.
        </Typography.Text>
        {expired && card.offer && (
          <div className="expired-offer">
            <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
              Предложение больше не действует
            </Typography.Title>
            <Typography.Text tag="p" view="primary-small">
              {products[card.offer.product]} · {money(card.offer.amount_kop)} · {card.offer.term_months} мес.
            </Typography.Text>
            <Typography.Text tag="p" view="primary-small">
              {rate(card.offer.rate_bp)} годовых · {money(card.offer.monthly_payment_kop)} в месяц
            </Typography.Text>
          </div>
        )}
        {expired && (
          <Link className="primary-link" to="/client/new" state={repeatApplication(card)}>
            Подать заново
          </Link>
        )}
      </section>
    );
  }
  if (card.status === 'rejected') {
    const group = (kind?: components['schemas']['Reason']['kind']) =>
      card.reasons.filter((reason) => reason.kind === kind).map((reason) => reason.text);
    const plan = [
      ['Что помешало', group('minus')],
      ['В вашу пользу', group('plus')],
      ['Что можно сделать', group('fix')],
    ] as const;
    return (
      <OutcomePanel rejected>
        <Typography.Title tag="h2" view="small" font="system" defaultMargins id="rejection-title">
          Банк не одобрил заявку
        </Typography.Title>
        {card.reasons.length ? (
          group(undefined).map((text) => (
            <Typography.Text tag="p" view="primary-small" key={text}>
              {text}
            </Typography.Text>
          ))
        ) : (
          <Typography.Text tag="p" view="primary-small">
            Банк пока не передал подробное объяснение.
          </Typography.Text>
        )}
        {plan.map(
          ([title, texts]) =>
            !!texts.length && (
              <div key={title} className="reject-plan">
                <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
                  {title}
                </Typography.Title>
                <ul>
                  {texts.map((text) => (
                    <li key={text}>{text}</li>
                  ))}
                </ul>
              </div>
            ),
        )}
        <Link className="primary-link" to="/client/new">
          Подать новую заявку
        </Link>
      </OutcomePanel>
    );
  }
  if (['issued', 'closed'].includes(card.status))
    return (
      <OutcomePanel>
        <Typography.Title tag="h2" view="small" font="system" defaultMargins id="success-title">
          Всё готово. Деньги на счёте!
        </Typography.Title>
        <Typography.Text tag="p" view="primary-small">
          Заявка одобрена, документы подписаны.
        </Typography.Text>
        <strong className="success-amount">{money(card.offer?.amount_kop ?? card.amount_kop)}</strong>
        <Typography.Text tag="p" view="primary-small">
          {products[card.offer?.product ?? card.product]} · {card.offer?.term_months ?? card.term_months} мес.
        </Typography.Text>
        <Typography.Text tag="p" view="primary-small" className="hint">
          Деньги на счёте. Выдача смоделирована, реального перевода нет.
        </Typography.Text>
      </OutcomePanel>
    );
  return (
    <motion.section layout={reduced ? false : 'position'} className="decision-panel">
      {card.status === 'counter_offer' ? (
        <>
          {!!card.reasons.filter((reason) => reason.kind !== 'plus').length && (
            <section className="counter-reasons">
              <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
                Почему банк предложил другие условия
              </Typography.Title>
              {card.reasons
                .filter((reason) => reason.kind !== 'plus')
                .map((reason, index) => (
                  <Typography.Text tag="p" view="primary-small" key={`${reason.code}-${index}`}>
                    {reason.text}
                  </Typography.Text>
                ))}
            </section>
          )}
          {!!card.reasons.filter((reason) => reason.kind === 'plus').length && (
            <section className="counter-positives">
              <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
                Что учтено в вашу пользу
              </Typography.Title>
              {card.reasons
                .filter((reason) => reason.kind === 'plus')
                .map((reason, index) => (
                  <Typography.Text tag="p" view="primary-small" key={`${reason.code}-${index}`}>
                    {reason.text}
                  </Typography.Text>
                ))}
            </section>
          )}
        </>
      ) : (
        <>
          {!!card.reasons.length && (
            <>
              <Typography.Title tag="h2" view="small" font="system" defaultMargins>
                {card.status === 'manual_review' ? 'Что учитывает специалист' : 'Почему принято такое решение'}
              </Typography.Title>
              {card.reasons.map((reason, index) => (
                <Typography.Text tag="p" view="primary-small" key={`${reason.code}-${index}`}>
                  {reason.text}
                </Typography.Text>
              ))}
            </>
          )}
        </>
      )}
      {card.offer && (
        <>
          <Typography.Title tag="h2" view="small" font="system" defaultMargins>
            {card.status === 'counter_offer' ? 'Предлагаем другие условия' : 'Условия финансирования'}
          </Typography.Title>
          {card.status === 'counter_offer' ? (
            <CounterOfferComparison key={card.id} card={card} />
          ) : (
            <>
              <div className="offer-highlights">
                <div>
                  <small>Банк предлагает</small>
                  <strong>
                    <Value text={money(card.offer.amount_kop)} />
                  </strong>
                </div>
                <div>
                  <small>Ставка в год</small>
                  <strong>{rate(card.offer.rate_bp)}</strong>
                </div>
                <div>
                  <small>Срок</small>
                  <strong>{card.offer.term_months} мес.</strong>
                </div>
                <div>
                  <small>Ежемесячный платёж</small>
                  <strong>
                    <Value text={money(card.offer.monthly_payment_kop)} />
                  </strong>
                </div>
              </div>
              <Disclosure className="offer-comparison" label={<>Сравнить с моей заявкой</>}>
                <div className="table-scroll">
                  <Table>
                    <Table.THead>
                      <Table.THeadCell>Параметр</Table.THeadCell>
                      <Table.THeadCell>Запрошено</Table.THeadCell>
                      <Table.THeadCell>Предложено</Table.THeadCell>
                    </Table.THead>
                    <Table.TBody>
                      <Table.TRow>
                        <Table.TCell>Продукт</Table.TCell>
                        <Table.TCell>{products[card.product]}</Table.TCell>
                        <Table.TCell>{products[card.offer.product]}</Table.TCell>
                      </Table.TRow>
                      <Table.TRow>
                        <Table.TCell>Сумма</Table.TCell>
                        <Table.TCell>{money(card.amount_kop)}</Table.TCell>
                        <Table.TCell>{money(card.offer.amount_kop)}</Table.TCell>
                      </Table.TRow>
                      <Table.TRow>
                        <Table.TCell>Срок</Table.TCell>
                        <Table.TCell>{card.term_months} мес.</Table.TCell>
                        <Table.TCell>{card.offer.term_months} мес.</Table.TCell>
                      </Table.TRow>
                    </Table.TBody>
                  </Table>
                </div>
              </Disclosure>
            </>
          )}
          {card.status !== 'counter_offer' && (
            <Typography.Text tag="p" view="primary-small">
              {rate(card.offer.rate_bp)} годовых. Платёж: {money(card.offer.monthly_payment_kop)}.
            </Typography.Text>
          )}
          {card.allowed_next.includes('accepted') && (
            <Typography.Text tag="p" view="primary-small">
              Предложение действительно до {new Date(card.offer.expires_at).toLocaleString('ru-RU')}.
            </Typography.Text>
          )}
          {card.offer.guarantor_required && (
            <Typography.Text tag="p" view="primary-small">
              Нужно поручительство собственника: он отвечает по обязательствам компании.
            </Typography.Text>
          )}
          {card.offer.pledge_required && (
            <Typography.Text tag="p" view="primary-small">
              Покупаемое оборудование потребуется передать в залог.
            </Typography.Text>
          )}
          {card.offer.msp_guarantee && (
            <Typography.Text tag="p" view="primary-small">
              Поручительство Корпорации МСП.
            </Typography.Text>
          )}
        </>
      )}
      {error && <BottomNotice message={error} onClose={() => setError('')} />}
      {offerExpired && (
        <InlineNotice>Срок предложения истёк. Обновите карточку, чтобы узнать актуальный статус.</InlineNotice>
      )}
      {card.allowed_next.includes('accepted') && !offerExpired && (
        <Button view="primary" loading={busy} onClick={() => action('accept')}>
          Принять условия
        </Button>
      )}
      {card.status === 'signing' && card.allowed_next.includes('issued') && (
        <>
          <Typography.Title tag="h2" view="small" font="system" defaultMargins>
            Подписание
          </Typography.Title>
          {card.due_at && (
            <Typography.Text tag="p" view="primary-small">
              Подписать нужно до {new Date(card.due_at).toLocaleTimeString('ru-RU')}.
            </Typography.Text>
          )}
          {contract && (
            <Typography.Text tag="p" view="primary-small">
              <a href={`/api/documents/${encodeURIComponent(contract.id)}/file`} target="_blank" rel="noreferrer">
                Прочитать договор и график платежей
              </a>
            </Typography.Text>
          )}
          <Typography.Text tag="p" view="primary-small">
            Код подписания придёт в уведомления кабинета. SMS в прототипе нет.
          </Typography.Text>
          <Button loading={busy} onClick={() => action('signing-code')}>
            {issued ? 'Получить новый код' : 'Получить код'}
          </Button>
          {codeExpired && <InlineNotice>Срок кода истёк. Получите новый код.</InlineNotice>}
          {issued && !codeExpired && (
            <form onSubmit={sign}>
              <Typography.Text tag="p" view="primary-small">
                Код пришёл в уведомления. Попыток: {issued.attempts_left}.
              </Typography.Text>
              <Deadline
                key={issued.expires_at}
                at={issued.expires_at}
                label="Срок действия кода"
                expired="Срок кода истёк. Получите новый код."
              />
              <Typography.Text tag="p" view="primary-small">
                Код подписания · 6 цифр
              </Typography.Text>
              <CodeInput key={issued?.expires_at} ref={codeInput} fields={6} onChange={setCode} disabled={busy} />
              <Button type="submit" view="primary" loading={busy} disabled={code.length !== 6}>
                Подписать
              </Button>
            </form>
          )}
        </>
      )}
    </motion.section>
  );
}

// отмена необратима, поэтому только через подтверждение и ниже основного действия карточки
export function Cancel({ card, refresh }: { card: App; refresh: () => void }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  if (!card.allowed_next.includes('cancelled_by_client')) return null;
  async function cancel() {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const { data, error, response } = await api.POST('/applications/{id}/cancel', {
        params: { path: { id: card.id } },
        body: { reason: 'Отмена клиентом в кабинете' },
      });
      if (data) {
        setOpen(false);
        notify('Заявка отменена.');
        refresh();
      } else
        setError(
          response.status === 409
            ? 'Состояние заявки изменилось. Закройте окно и обновите карточку.'
            : errorText(error, 'Не удалось отменить заявку.'),
        );
    } catch {
      setError('Нет связи с сервером. Проверьте статус перед повтором.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Button view="secondary" size={40} onClick={() => setOpen(true)}>
        Отменить заявку
      </Button>
      <Modal open={open} onClose={() => !busy && setOpen(false)} size={500}>
        <Modal.Header hasCloser title={`Отменить заявку № ${card.number}?`} />
        <Modal.Content>
          <Typography.Text tag="p" view="primary-small">
            Вернуть её будет нельзя. Чтобы получить финансирование, придётся подать новую заявку.
          </Typography.Text>
          {card.offer && (
            <Typography.Text tag="p" view="primary-small">
              Условия, которые предложил банк, перестанут действовать.
            </Typography.Text>
          )}
          {error && <InlineNotice>{error}</InlineNotice>}
        </Modal.Content>
        <Modal.Footer>
          <Modal.Controls
            primary={
              <Button view="primary" loading={busy} onClick={cancel}>
                Да, отменить
              </Button>
            }
            secondary={
              <Button view="secondary" disabled={busy} onClick={() => setOpen(false)}>
                Оставить заявку
              </Button>
            }
          />
        </Modal.Footer>
      </Modal>
    </>
  );
}
