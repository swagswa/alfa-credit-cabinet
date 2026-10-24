import { money, rate, products } from '../api/presentation';
import { isCompleted } from '../api/dashboard';
import { Value } from '../motion/Value';
import { Button, MotionDetails, motion, useMotionDisabled } from '../motion';
import { OutcomePanel } from '../motion/OutcomePanel';
import { BottomNotice, notify } from '../BottomNotice';
import { useEffect, useState, type FormEvent } from 'react';
import { Input } from '@alfalab/core-components/input';
import { Modal } from '@alfalab/core-components/modal';
import { api, errorText } from '../api/client';
import type { components } from '../api/schema.gen';
type App = components['schemas']['ApplicationView'];
export function Decision({ card, refresh }: { card: App; refresh: () => void }) {
  const reduced = useMotionDisabled();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [code, setCode] = useState('');
  const [issued, setIssued] = useState<components['schemas']['SigningCode'] | null>(null);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (isCompleted(card.status) || (!issued && !card.offer)) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [card.status, card.offer, issued]);
  const codeExpired = !!issued && Date.parse(issued.expires_at) <= now;
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
        if ('code' in result.data) {
          setIssued(result.data);
          setCode('');
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
    if (busy || codeExpired) return;
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
        }
      }
    } catch {
      setError('Нет связи с сервером. Проверьте статус перед повтором.');
    } finally {
      setBusy(false);
    }
  }
  if (card.status === 'rejected')
    return (
      <OutcomePanel rejected>
        <h2 id="rejection-title">Банк не одобрил заявку</h2>
        {card.reasons.length ? (
          card.reasons.map((reason, i) => <p key={`${reason.code}-${i}`}>{reason.text}</p>)
        ) : (
          <p>Банк пока не передал подробное объяснение.</p>
        )}
      </OutcomePanel>
    );
  if (['issued', 'closed'].includes(card.status))
    return (
      <OutcomePanel>
        <h2 id="success-title">Всё готово. Деньги на счёте!</h2>
        <p>Заявка одобрена, документы подписаны.</p>
        <strong className="success-amount">{money(card.offer?.amount_kop ?? card.amount_kop)}</strong>
        <p>
          {products[card.offer?.product ?? card.product]} · {card.offer?.term_months ?? card.term_months} мес.
        </p>
        <p className="hint">Деньги на счёте. Выдача смоделирована, реального перевода нет.</p>
      </OutcomePanel>
    );
  return (
    <motion.section layout={reduced ? false : 'position'} className="decision-panel">
      {!!card.reasons.length && (
        <>
          <h2>
            {card.status === 'need_info'
              ? 'Что нужно уточнить'
              : card.status === 'manual_review'
                ? 'Что учитывает специалист'
                : 'Почему принято такое решение'}
          </h2>
          {card.reasons.map((reason, index) => (
            <p key={`${reason.code}-${index}`}>{reason.text}</p>
          ))}
        </>
      )}
      {card.offer && (
        <>
          <h2>{card.status === 'counter_offer' ? 'Предлагаем другие условия' : 'Условия финансирования'}</h2>
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
          <MotionDetails className="offer-comparison">
            <summary>Сравнить с моей заявкой</summary>
            <div className="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>Параметр</th>
                    <th>Запрошено</th>
                    <th>Предложено</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <th>Продукт</th>
                    <td>{products[card.product]}</td>
                    <td>{products[card.offer.product]}</td>
                  </tr>
                  <tr>
                    <th>Сумма</th>
                    <td>{money(card.amount_kop)}</td>
                    <td>{money(card.offer.amount_kop)}</td>
                  </tr>
                  <tr>
                    <th>Срок</th>
                    <td>{card.term_months} мес.</td>
                    <td>{card.offer.term_months} мес.</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </MotionDetails>
          <p>
            {rate(card.offer.rate_bp)} годовых. Платёж: {money(card.offer.monthly_payment_kop)}.
          </p>
          {card.allowed_next.includes('accepted') && (
            <p>Предложение действительно до {new Date(card.offer.expires_at).toLocaleString('ru-RU')}.</p>
          )}
          {card.offer.guarantor_required && (
            <p>Нужно поручительство собственника: он отвечает по обязательствам компании.</p>
          )}
          {card.offer.pledge_required && <p>Покупаемое оборудование потребуется передать в залог.</p>}
          {card.offer.msp_guarantee && <p>Поручительство Корпорации МСП.</p>}
        </>
      )}
      {error && <BottomNotice message={error} onClose={() => setError('')} />}
      {offerExpired && <p role="alert">Срок предложения истёк. Обновите карточку, чтобы узнать актуальный статус.</p>}
      {card.allowed_next.includes('accepted') && !offerExpired && (
        <Button view="primary" loading={busy} onClick={() => action('accept')}>
          Принять условия
        </Button>
      )}
      {card.status === 'signing' && card.allowed_next.includes('issued') && (
        <>
          <h2>Подписание</h2>
          <p>Учебное подписание: код показывается здесь, отправки SMS нет.</p>
          <Button loading={busy} onClick={() => action('signing-code')}>
            Получить новый код
          </Button>
          {codeExpired && <p role="alert">Срок кода истёк. Получите новый код.</p>}
          {issued && !codeExpired && (
            <form onSubmit={sign}>
              <p>
                Код: {issued.code}. Действует до {new Date(issued.expires_at).toLocaleTimeString('ru-RU')}. Попыток:{' '}
                {issued.attempts_left}.
              </p>
              <Input
                block
                label="Код подписания"
                value={code}
                required
                pattern="[0-9]{6}"
                maxLength={6}
                onChange={(event) => setCode(event.target.value)}
                disabled={busy}
              />
              <Button type="submit" view="primary" loading={busy}>
                Подписать
              </Button>
            </form>
          )}
        </>
      )}
      {['issued', 'closed'].includes(card.status) && (
        <p>Деньги на счёте. Выдача смоделирована, реального перевода нет.</p>
      )}
      {card.status === 'expired' && <p>Срок ответа истёк. Можно создать новую заявку.</p>}
      {card.status === 'cancelled_by_client' && <p>Вы отменили заявку. Можно подать новую.</p>}
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
          <p>Вернуть её будет нельзя. Чтобы получить финансирование, придётся подать новую заявку.</p>
          {card.offer && <p>Условия, которые предложил банк, перестанут действовать.</p>}
          {error && <p role="alert">{error}</p>}
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
